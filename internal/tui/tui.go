// Package tui provides a BubbleTea-based live dashboard for RaceLLM.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/khuynh22/racellm/internal/coordinator"
	"github.com/khuynh22/racellm/internal/provider"
)

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	runningStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	winnerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	doneStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("70"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	cancelledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type raceEventMsg struct{ e coordinator.RaceEvent }
type eventsDoneMsg struct{}
type resultsMsg struct{ results []provider.Result }

type racerStatus int

const (
	statusRunning   racerStatus = iota
	statusFinished              // completed but not the winner
	statusWinner                // first to finish (or fastest overall)
	statusError                 // encountered an error
	statusCancelled             // stopped because another racer won
)

type racerState struct {
	label      string
	tokenCount int
	totalTime  time.Duration
	ttft       time.Duration
	status     racerStatus
	err        error
}

// Model is the BubbleTea application model for the race dashboard.
type Model struct {
	racers      []*racerState
	racerIdx    map[string]int // "Provider/Model" -> index in racers slice
	prompt      string
	mode        coordinator.RaceMode
	results     []provider.Result
	done        bool
	cancel      context.CancelFunc
	eventChan   <-chan coordinator.RaceEvent
	resultsChan <-chan []provider.Result
	width       int
}

// New builds a Model from the coordinator's outputs and race parameters.
func New(
	cancel context.CancelFunc,
	entrants []coordinator.Entrant,
	prompt string,
	mode coordinator.RaceMode,
	eventChan <-chan coordinator.RaceEvent,
	resultsChan <-chan []provider.Result,
) Model {
	racers := make([]*racerState, len(entrants))
	racerIdx := make(map[string]int, len(entrants))
	for i, e := range entrants {
		label := fmt.Sprintf("%s/%s", e.Provider.Name(), e.Model)
		racers[i] = &racerState{label: label, status: statusRunning}
		racerIdx[label] = i
	}
	return Model{
		racers:      racers,
		racerIdx:    racerIdx,
		prompt:      prompt,
		mode:        mode,
		cancel:      cancel,
		eventChan:   eventChan,
		resultsChan: resultsChan,
		width:       80,
	}
}

// Init implements tea.Model and returns the initial command to start listening for race events.
func (m Model) Init() tea.Cmd {
	return listenForEvent(m.eventChan)
}

// Update implements tea.Model and processes incoming messages to update race state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}

	case raceEventMsg:
		e := msg.e
		switch e.Type {
		case coordinator.EventToken:
			if idx, ok := m.racerIdx[e.Entrant]; ok {
				m.racers[idx].tokenCount++
			}

		case coordinator.EventFinish:
			if idx, ok := m.racerIdx[e.Entrant]; ok {
				r := m.racers[idx]
				if e.Result != nil {
					r.totalTime = e.Result.TotalTime
					r.ttft = e.Result.TTFT
					r.tokenCount = e.Result.TokenCount
					switch {
					case e.Result.Canceled:
						r.status = statusCancelled
					case e.Result.Err != nil:
						r.status = statusError
						r.err = e.Result.Err
					case r.status != statusWinner:
						r.status = statusFinished
					}
				}
			}

		case coordinator.EventWinner:
			if idx, ok := m.racerIdx[e.Entrant]; ok {
				r := m.racers[idx]
				r.status = statusWinner
				if e.Result != nil {
					r.totalTime = e.Result.TotalTime
					r.ttft = e.Result.TTFT
					r.tokenCount = e.Result.TokenCount
				}
			}

		case coordinator.EventError:
			if idx, ok := m.racerIdx[e.Entrant]; ok {
				m.racers[idx].status = statusError
				m.racers[idx].err = e.Err
			}
		}
		return m, listenForEvent(m.eventChan)

	case eventsDoneMsg:
		return m, listenForResults(m.resultsChan)

	case resultsMsg:
		m.results = msg.results
		m.done = true
		for i, r := range msg.results {
			label := fmt.Sprintf("%s/%s", r.Provider, r.Model)
			idx, ok := m.racerIdx[label]
			if !ok {
				continue
			}
			racer := m.racers[idx]
			racer.totalTime = r.TotalTime
			racer.ttft = r.TTFT
			racer.tokenCount = r.TokenCount
			switch {
			case r.Canceled:
				racer.status = statusCancelled
			case r.Err != nil:
				racer.status = statusError
				racer.err = r.Err
			case i == 0:
				racer.status = statusWinner
			case racer.status != statusWinner:
				racer.status = statusFinished
			}
		}
		return m, tea.Quit
	}
	return m, nil
}

const (
	trackLen         = 36  // inner characters for the progress bar
	maxTokensForFull = 250 // token count that fills the track
	labelColWidth    = 26  // display width of the racer label column
)

// View implements tea.Model and renders the current race dashboard as a string.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("🏁 RaceLLM") + "\n")
	modeStr := "all"
	if m.mode == coordinator.ModeFastest {
		modeStr = "fastest"
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("   %d racers  ·  mode: %s", len(m.racers), modeStr)) + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("   prompt: %q", truncate(m.prompt, 60))) + "\n")
	b.WriteString("\n")

	for _, r := range m.racers {
		b.WriteString(m.renderRow(r) + "\n")
	}

	b.WriteString("\n")
	if m.done {
		b.WriteString(dimStyle.Render("   Race complete — full response below.") + "\n")
	} else {
		b.WriteString(dimStyle.Render("   q / ctrl+c to quit") + "\n")
	}

	return b.String()
}

func (m Model) renderRow(r *racerState) string {
	filled := 0
	if r.tokenCount > 0 {
		filled = r.tokenCount * trackLen / maxTokensForFull
		if filled > trackLen {
			filled = trackLen
		}
	}

	track := "[" + strings.Repeat("█", filled) + strings.Repeat("░", trackLen-filled) + "]"

	var icon, info string
	switch r.status {
	case statusRunning:
		icon = "▶"
		info = fmt.Sprintf("%4d tok", r.tokenCount)
	case statusWinner:
		icon = "★"
		info = fmt.Sprintf("%4d tok  %s  TTFT %s  WINNER",
			r.tokenCount,
			coordinator.FormatDuration(r.totalTime),
			coordinator.FormatDuration(r.ttft),
		)
	case statusFinished:
		icon = "✓"
		info = fmt.Sprintf("%4d tok  %s  TTFT %s",
			r.tokenCount,
			coordinator.FormatDuration(r.totalTime),
			coordinator.FormatDuration(r.ttft),
		)
	case statusError:
		icon = "✗"
		errStr := "error"
		if r.err != nil {
			errStr = r.err.Error()
			if len(errStr) > 36 {
				errStr = errStr[:33] + "..."
			}
		}
		info = "ERR: " + errStr
	case statusCancelled:
		icon = "◼"
		info = "CANCELED"
	}

	var style lipgloss.Style
	switch r.status {
	case statusRunning:
		style = runningStyle
	case statusWinner:
		style = winnerStyle
	case statusFinished:
		style = doneStyle
	case statusError:
		style = errorStyle
	case statusCancelled:
		style = cancelledStyle
	}

	labelStr := lipgloss.NewStyle().
		Inherit(style).
		Width(labelColWidth).
		Render(r.label)

	return "   " + labelStr + " " + style.Render(track) + " " + style.Render(icon) + "  " + style.Render(info)
}

func listenForEvent(c <-chan coordinator.RaceEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-c
		if !ok {
			return eventsDoneMsg{}
		}
		return raceEventMsg{event}
	}
}

func listenForResults(c <-chan []provider.Result) tea.Cmd {
	return func() tea.Msg {
		return resultsMsg{<-c}
	}
}

// Run starts the BubbleTea TUI, drives the race dashboard to completion, and
// returns the sorted final results. If the user quits early, cancel is called
// so the coordinator goroutines stop cleanly.
func Run(
	cancel context.CancelFunc,
	entrants []coordinator.Entrant,
	prompt string,
	mode coordinator.RaceMode,
	eventChan <-chan coordinator.RaceEvent,
	resultsChan <-chan []provider.Result,
) ([]provider.Result, error) {
	m := New(cancel, entrants, prompt, mode, eventChan, resultsChan)
	p := tea.NewProgram(m)
	fm, err := p.Run()
	if err != nil {
		return nil, err
	}
	m2, ok := fm.(Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type returned from BubbleTea")
	}
	return m2.results, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
