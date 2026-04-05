package main

import (
	"fmt"
	"image/color"
	"log"
	"math/rand"
	"os"
	"sync"

	gamma "github.com/PeronGH/gamma"
	"github.com/PeronGH/gamma/screen"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

type App struct {
	scr   *gamma.Window
	root  *gamma.Window
	mtx   sync.RWMutex
	quit  bool
	drag  *gamma.Window // window being dragged
	styles map[*gamma.Window]gamma.Style
}

func NewApp(width, height int) *App {
	a := new(App)
	a.scr = gamma.NewScreen(width, height)
	a.scr.SetWidthMethod(ansi.GraphemeWidth)
	a.root = a.scr.NewWindow(0, 0, width, height)
	a.styles = make(map[*gamma.Window]gamma.Style)
	return a
}

func (a *App) CreateWindow(x, y, width, height int) *gamma.Window {
	a.mtx.Lock()
	defer a.mtx.Unlock()

	var style gamma.Style
	style.Bg = ansi.IndexedColor(rand.Intn(256))

	win := a.root.NewWindow(x, y, width, height)
	win.Fill(&gamma.Cell{Content: " ", Width: 1, Style: style})
	a.styles[win] = style
	return win
}

func (a *App) DestroyWindow(win *gamma.Window) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.root.RemoveChild(win)
	delete(a.styles, win)
}

func (a *App) Draw(scr gamma.Screen, area gamma.Rectangle) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	screen.Clear(a.root)
	// Window.Draw now composites children in z-order automatically.
	a.root.Draw(scr, area)
}

func (a *App) HandleEvent(ev gamma.Event) bool {
	switch ev := ev.(type) {
	case gamma.KeyPressEvent:
		switch {
		case ev.MatchString("ctrl+c", "esc"):
			a.quit = true
			return true
		case len(ev.Text) > 0:
			// Type into the topmost window that was last clicked.
			if a.drag != nil {
				ctx := screen.NewContext(a.drag)
				st := a.styles[a.drag]
				ctx.SetStyle(st)
				ctx.SetForeground(color.Black)
				ctx.Print(ev.Text)
				return true
			}
		}

	case gamma.MouseMotionEvent:
		if a.drag != nil {
			bounds := a.drag.Bounds()
			newX := ev.X - bounds.Dx()/2
			newY := ev.Y - bounds.Dy()/2
			a.drag.MoveTo(newX, newY)
			return true
		}

	case gamma.MouseReleaseEvent:
		a.drag = nil
		return true

	case gamma.MouseClickEvent:
		switch ev.Button {
		case gamma.MouseLeft:
			// Hit test using Window.WindowAt.
			hit := a.root.WindowAt(ev.X, ev.Y)
			if hit != a.root {
				a.root.BringToFront(hit)
				a.drag = hit
				return true
			}

			// Clicked root -- create a new window.
			rootSize := a.root.Bounds().Size()
			width := rand.Intn(20)
			height := rand.Intn(10)
			if width < 3 || height < 2 {
				width, height = 8, 4
			}

			x := ev.X - width/2
			y := ev.Y - height/2
			x = max(0, min(x, rootSize.X-width))
			y = max(0, min(y, rootSize.Y-height))

			a.CreateWindow(x, y, width, height)
			return true

		case gamma.MouseRight:
			hit := a.root.WindowAt(ev.X, ev.Y)
			if hit != a.root {
				a.DestroyWindow(hit)
			}
			return true
		}
	}

	return false
}

func (a *App) Resize(width, height int) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.scr.Resize(width, height)
	a.root.Resize(width, height)
}

func (a *App) Run(input gamma.File, output gamma.File, environ []string) error {
	t := gamma.NewTerminal(gamma.NewConsole(input, output, environ), nil)
	scr := t.Screen()
	scr.EnterAltScreen()
	scr.HideCursor()

	if err := t.Start(); err != nil {
		return fmt.Errorf("failed to start terminal: %w", err)
	}
	defer t.Stop()

	scr.SetMouseMode(gamma.MouseModeDrag)

	for !a.quit {
		select {
		case ev := <-t.Events():
			switch ev := ev.(type) {
			case gamma.WindowSizeEvent:
				scr.Resize(ev.Width, ev.Height)
				a.Resize(ev.Width, ev.Height)
			}

			a.HandleEvent(ev)

			if err := scr.Display(a); err != nil {
				return fmt.Errorf("failed to display terminal: %w", err)
			}
		}
	}

	return nil
}

func init() {
	f, err := os.OpenFile("gamma.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	stdin, stdout, environ := os.Stdin, os.Stdout, os.Environ()
	physicalWidth, physicalHeight, err := term.GetSize(stdout.Fd())
	if err != nil {
		log.Fatalf("failed to get terminal size: %v", err)
	}

	app := NewApp(physicalWidth, physicalHeight)
	if err := app.Run(stdin, stdout, environ); err != nil {
		log.Fatalf("application error: %v", err)
	}
}
