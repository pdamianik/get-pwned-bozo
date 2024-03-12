package main

// An example Bubble Tea server. This will put an ssh session into alt screen
// and continually print up to date terminal information.

import (
	"context"
	"errors"
	"fmt"
	"github.com/muesli/termenv"
	"github.com/teacat/noire"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

const (
	host     = "0.0.0.0"
	port     = "22"
	step     = 40.0
	gradient = 6.0
	angle    = 6.0
)

const graphic = `⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⢠⡾⠲⠶⣤⣀⣠⣤⣤⣤⡿⠛⠿⡴⠾⠛⢻⡆⠀⠀⠀  _____      __    ___                        __  
⠀⠀⠀⣼⠁⠀⠀ ⠉⠁⠀⢀⣿⠐⡿⣿⠿⣶⣤⣤⣷⡀⠀⠀⠀/ ___/___  / /_  / _ \ _    __ ___  ___  ___/ /  
⠀⠀⠀⢹⡶⠀⠀⠀⠀⠀⠀⠈⢯⣡⣿⣿⣀⣸⣿⣦⢓⡟⠀⠀/ (_ // -_)/ __/ / ___/| |/|/ // _ \/ -_)/ _  /   
⠀⠀⢀⡿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠹⣍⣭⣾⠁⠀ \___/ \__/ \__/ /_/    |__,__//_//_/\__/ \_,_/⠀
⠀⣀⣸⣇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣸⣷⣤⡀   ___                       
⠈⠉⠹⣏⡁⠀⢸⣿⠀⠀⠀⢀⡀⠀⠀⠀⣿⠆⠀⢀⣸⣇⣀⠀  / _ ) ___  ___ ___ ___      
⠀⠐⠋⢻⣅⣄⢀⣀⣀⡀⠀⠯⠽⠂⢀⣀⣀⡀⠀⣤⣿⠀⠉⠀ / _  |/ _ \/_ //_ // _ \     
⠀⠀⠴⠛⠙⣳⠋⠉⠉⠙⣆⠀⠀⢰⡟⠉⠈⠙⢷⠟⠉⠙⠂⠀/____/ \___//__//__/\___/     
⠀⠀⠀⠀⠀⢻⣄⣠⣤⣴⠟⠛⠛⠛⢧⣤⣤⣀⡾⠁⠀⠀⠀⠀
`

func main() {
	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			myCustomBubbleteaMiddleware(),
			activeterm.Middleware(), // Bubble Tea apps usually require a PTY.
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("Starting SSH server", "host", host, "port", port)
	go func() {
		if err = s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server", "error", err)
	}
}

func myCustomBubbleteaMiddleware() wish.Middleware {
	newProg := func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		p := tea.NewProgram(m, opts...)
		go func() {
			var tick uint = 0
			for {
				tick++
				p.Send(tickMsg(tick))
				<-time.After(100 * time.Millisecond)
			}
		}()
		return p
	}

	teaHandler := func(s ssh.Session) *tea.Program {
		// This should never fail, as we are using the activeterm middleware.
		pty, _, _ := s.Pty()

		color := noire.NewHSV(0, 66, 100)
		// When running a Bubble Tea app over SSH, you shouldn't use the default
		// lipgloss.NewStyle function.
		// That function will use the color profile from the os.Stdin, which is the
		// server, not the client.
		// We provide a MakeRenderer function in the bubbletea middleware package,
		// so you can easily get the correct renderer for the current session, and
		// use it to create the styles.
		// The recommended way to use these styles is to then pass them down to
		// your Bubble Tea model.
		renderer := bubbletea.MakeRenderer(s)
		style := renderer.NewStyle()
		txtStyle := renderer.NewStyle().Foreground(lipgloss.Color("10"))
		quitStyle := renderer.NewStyle().Foreground(lipgloss.Color("8"))
		address := s.RemoteAddr()

		m := model{
			term:      pty.Term,
			address:   address,
			width:     pty.Window.Width,
			height:    pty.Window.Height,
			color:     color,
			style:     style,
			txtStyle:  txtStyle,
			quitStyle: quitStyle,
		}
		return newProg(m, append(bubbletea.MakeOptions(s), tea.WithAltScreen())...)
	}
	return bubbletea.MiddlewareWithProgramHandler(teaHandler, termenv.ANSI256)
}

// Just a generic tea.Model to demo terminal information of ssh.
type model struct {
	term      string
	address   net.Addr
	width     int
	height    int
	tick      uint
	color     noire.Color
	style     lipgloss.Style
	txtStyle  lipgloss.Style
	quitStyle lipgloss.Style
}

type tickMsg uint

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		m.tick = uint(msg)
		m.color = m.color.AdjustHue(step)
	}
	return m, nil
}

func (m model) View() string {
	msg := fmt.Sprintf("Your IP is %v", m.address.(*net.TCPAddr).IP)
	return lolcat(graphic, &m.color, m.style) + "\n" + m.txtStyle.Render(msg) + "\n" + m.quitStyle.Render("Press 'q' to quit\n")
}

func lolcat(msg string, initialColor *noire.Color, style lipgloss.Style) string {
	builder := strings.Builder{}
	rowColor := *initialColor
	charColor := rowColor
	for _, c := range []rune(msg) {
		if c == '\n' {
			builder.WriteRune(c)
			rowColor = rowColor.AdjustHue(angle)
			charColor = rowColor
			continue
		}
		builder.WriteString(style.Foreground(noireColorToLipglossColor(charColor)).Render(string(c)))
		charColor = charColor.AdjustHue(gradient)
	}
	return builder.String()
}

func noireColorToLipglossColor(color noire.Color) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%s", color.Hex()))
}
