package installer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type page int

const (
	pageWelcome page = iota
	pageServer
	pageDomains
	pageAdmin
	pageMarketplaces
	pageReview
	pageConsent
	pageInstalling
	pageDone
)

type TUIOptions struct {
	Run func(context.Context, Config, Progress) error
}

func RunTUI(ctx context.Context, opts TUIOptions) error {
	m := newModel(ctx, opts)
	_, err := tea.NewProgram(m).Run()
	return err
}

type model struct {
	ctx      context.Context
	cfg      Config
	page     page
	inputs   []textinput.Model
	focus    int
	err      string
	events   []StepEvent
	progress chan progressMsg
	run      func(context.Context, Config, Progress) error
}

type progressMsg struct {
	Event StepEvent
	Done  bool
	Err   error
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func newModel(ctx context.Context, opts TUIOptions) model {
	cfg := DefaultConfig()
	m := model{ctx: ctx, cfg: cfg, page: pageWelcome, run: opts.Run}
	if m.run == nil {
		m.run = func(ctx context.Context, cfg Config, progress Progress) error {
			sshExec, err := NewSSHExecutor(cfg)
			if err != nil {
				return err
			}
			defer sshExec.Close()
			admin, err := NewHTTPAdminClient(cfg.BaseURL())
			if err != nil {
				return err
			}
			return Run(ctx, cfg, sshExec, NetResolver{}, admin, progress)
		}
	}
	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressMsg:
		if msg.Event.Name != "" {
			m.events = append(m.events, msg.Event)
		}
		if msg.Done {
			m.page = pageDone
			if msg.Err != nil {
				m.err = MaskSecrets(msg.Err.Error(), SecretValues(m.cfg))
			}
			return m, nil
		}
		return m, m.waitProgress()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
		if m.page == pageWelcome && msg.String() == "enter" {
			m.page = pageServer
			m.buildInputs()
			return m, nil
		}
		if m.page == pageReview && (msg.String() == "u" || msg.String() == "U") {
			m.cfg.Deploy.AutoUpdate = !m.cfg.Deploy.AutoUpdate
			return m, nil
		}
		if m.page == pageReview && msg.String() == "enter" {
			if err := m.cfg.Validate(); err != nil {
				m.err = err.Error()
				return m, nil
			}
			m.err = ""
			m.page = pageConsent
			return m, nil
		}
		if m.page == pageConsent {
			// Installation is unreachable without explicit consent: only "y"
			// proceeds; "q" quits; any other navigation key is a no-op.
			if msg.String() == "q" || msg.String() == "Q" {
				return m, tea.Quit
			}
			if msg.String() == "y" || msg.String() == "Y" {
				m.err = ""
				m.page = pageInstalling
				m.events = nil
				m.progress = make(chan progressMsg, 32)
				return m, tea.Batch(m.startInstall(), m.waitProgress())
			}
			return m, nil
		}
		if m.page == pageDone && msg.String() == "enter" {
			return m, tea.Quit
		}
		if m.page == pageServer && msg.String() == "a" {
			if m.cfg.Server.AuthMethod == SSHAuthPassword {
				m.cfg.Server.AuthMethod = SSHAuthKey
			} else {
				m.cfg.Server.AuthMethod = SSHAuthPassword
			}
			m.buildInputs()
			return m, nil
		}
		if m.page == pageMarketplaces {
			switch msg.String() {
			case "w":
				m.saveInputs()
				m.cfg.Marketplaces.WB.Enabled = !m.cfg.Marketplaces.WB.Enabled
				m.buildInputs()
				return m, nil
			case "y":
				m.saveInputs()
				m.cfg.Marketplaces.YM.Enabled = !m.cfg.Marketplaces.YM.Enabled
				m.buildInputs()
				return m, nil
			case "o":
				m.saveInputs()
				m.cfg.Marketplaces.Ozon.Enabled = !m.cfg.Marketplaces.Ozon.Enabled
				m.buildInputs()
				return m, nil
			}
		}
		if len(m.inputs) > 0 {
			switch msg.String() {
			case "tab", "down":
				m.moveFocus(1)
				return m, nil
			case "shift+tab", "up":
				m.moveFocus(-1)
				return m, nil
			case "enter":
				m.saveInputs()
				if m.page < pageReview {
					m.page++
					m.buildInputs()
				}
				return m, nil
			case "backspace":
				// Let text inputs handle backspace below.
			}
			var cmd tea.Cmd
			m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *model) moveFocus(delta int) {
	if len(m.inputs) == 0 {
		return
	}
	m.inputs[m.focus].Blur()
	m.focus = (m.focus + delta + len(m.inputs)) % len(m.inputs)
	m.inputs[m.focus].Focus()
}

func (m *model) buildInputs() {
	m.saveInputs()
	m.focus = 0
	switch m.page {
	case pageServer:
		fields := []inputSpec{
			{"VPS IP / host", m.cfg.Server.Host, false},
			{"SSH port", strconv.Itoa(m.cfg.Server.Port), false},
			{"SSH user", m.cfg.Server.User, false},
		}
		if m.cfg.Server.AuthMethod == SSHAuthPassword {
			fields = append(fields, inputSpec{"SSH password", m.cfg.Server.Password, true})
		} else {
			fields = append(fields, inputSpec{"Private key path", m.cfg.Server.KeyPath, false}, inputSpec{"Key passphrase (optional)", m.cfg.Server.KeyPassphrase, true})
		}
		fields = append(fields, inputSpec{"Sudo password (optional)", m.cfg.Server.SudoPassword, true})
		m.inputs = makeInputs(fields)
	case pageDomains:
		m.inputs = makeInputs([]inputSpec{
			{"Reviews domain", m.cfg.Domains.ReviewsDomain, false},
			{"Shop origin", m.cfg.Domains.ShopOrigin, false},
			{"Sitemap URL (optional)", m.cfg.Domains.SitemapURL, false},
		})
	case pageAdmin:
		m.inputs = makeInputs([]inputSpec{
			{"Admin login", m.cfg.Admin.Login, false},
			{"Admin password", m.cfg.Admin.Password, true},
			{"Confirm admin password", m.cfg.Admin.PasswordConfirm, true},
		})
	case pageMarketplaces:
		fields := []inputSpec{}
		if m.cfg.Marketplaces.WB.Enabled {
			fields = append(fields, inputSpec{"WB token", m.cfg.Marketplaces.WB.Token, true})
		}
		if m.cfg.Marketplaces.YM.Enabled {
			fields = append(fields,
				inputSpec{"YM API key", m.cfg.Marketplaces.YM.APIKey, true},
				inputSpec{"YM OAuth token (optional)", m.cfg.Marketplaces.YM.OAuthToken, true},
				inputSpec{"YM Business ID", m.cfg.Marketplaces.YM.BusinessID, false},
				inputSpec{"YM Campaign ID (optional)", m.cfg.Marketplaces.YM.CampaignID, false},
			)
		}
		if m.cfg.Marketplaces.Ozon.Enabled {
			fields = append(fields, inputSpec{"Ozon Client ID", m.cfg.Marketplaces.Ozon.ClientID, false}, inputSpec{"Ozon API key", m.cfg.Marketplaces.Ozon.APIKey, true})
		}
		if len(fields) == 0 {
			fields = append(fields, inputSpec{"No marketplace fields enabled; press w/y/o to enable", "", false})
		}
		m.inputs = makeInputs(fields)
	default:
		m.inputs = nil
	}
	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
}

type inputSpec struct {
	Placeholder string
	Value       string
	Secret      bool
}

func makeInputs(specs []inputSpec) []textinput.Model {
	inputs := make([]textinput.Model, len(specs))
	for i, spec := range specs {
		in := textinput.New()
		in.Placeholder = spec.Placeholder
		in.SetValue(spec.Value)
		in.CharLimit = 4096
		in.Width = 72
		if spec.Secret {
			in.EchoMode = textinput.EchoPassword
			in.EchoCharacter = '•'
		}
		inputs[i] = in
	}
	return inputs
}

func (m *model) saveInputs() {
	if len(m.inputs) == 0 {
		return
	}
	values := make([]string, len(m.inputs))
	for i := range m.inputs {
		values[i] = strings.TrimSpace(m.inputs[i].Value())
	}
	switch m.page {
	case pageServer:
		if len(values) >= 3 {
			m.cfg.Server.Host = values[0]
			if port, err := strconv.Atoi(values[1]); err == nil {
				m.cfg.Server.Port = port
			}
			m.cfg.Server.User = values[2]
		}
		if m.cfg.Server.AuthMethod == SSHAuthPassword {
			if len(values) >= 4 {
				m.cfg.Server.Password = m.inputs[3].Value()
			}
			if len(values) >= 5 {
				m.cfg.Server.SudoPassword = m.inputs[4].Value()
			}
		} else {
			if len(values) >= 5 {
				m.cfg.Server.KeyPath = values[3]
				m.cfg.Server.KeyPassphrase = m.inputs[4].Value()
			}
			if len(values) >= 6 {
				m.cfg.Server.SudoPassword = m.inputs[5].Value()
			}
		}
	case pageDomains:
		m.cfg.Domains.ReviewsDomain = values[0]
		m.cfg.Domains.ShopOrigin = strings.TrimRight(values[1], "/")
		m.cfg.Domains.SitemapURL = values[2]
	case pageAdmin:
		m.cfg.Admin.Login = values[0]
		m.cfg.Admin.Password = m.inputs[1].Value()
		m.cfg.Admin.PasswordConfirm = m.inputs[2].Value()
	case pageMarketplaces:
		i := 0
		if m.cfg.Marketplaces.WB.Enabled && i < len(m.inputs) {
			m.cfg.Marketplaces.WB.Token = m.inputs[i].Value()
			i++
		}
		if m.cfg.Marketplaces.YM.Enabled && i+3 < len(m.inputs) {
			m.cfg.Marketplaces.YM.APIKey = m.inputs[i].Value()
			m.cfg.Marketplaces.YM.OAuthToken = m.inputs[i+1].Value()
			m.cfg.Marketplaces.YM.BusinessID = values[i+2]
			m.cfg.Marketplaces.YM.CampaignID = values[i+3]
			i += 4
		}
		if m.cfg.Marketplaces.Ozon.Enabled && i+1 < len(m.inputs) {
			m.cfg.Marketplaces.Ozon.ClientID = values[i]
			m.cfg.Marketplaces.Ozon.APIKey = m.inputs[i+1].Value()
		}
	}
}

func (m model) startInstall() tea.Cmd {
	cfg := m.cfg
	return func() tea.Msg {
		go func() {
			progress := func(event StepEvent) {
				m.progress <- progressMsg{Event: event}
			}
			err := m.run(m.ctx, cfg, progress)
			m.progress <- progressMsg{Done: true, Err: err}
			close(m.progress)
		}()
		return nil
	}
}

func (m model) waitProgress() tea.Cmd {
	ch := m.progress
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return progressMsg{Done: true}
		}
		return msg
	}
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Reviews installer") + "\n\n")
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n\n")
	}
	switch m.page {
	case pageWelcome:
		b.WriteString("This wizard installs Reviews on a fresh Ubuntu/Debian VPS.\n\n")
		b.WriteString("Prepare: VPS IP, SSH port, root/sudo login, SSH password or key,\n")
		b.WriteString("reviews domain pointed to VPS IP, shop origin, and marketplace keys.\n\n")
		b.WriteString("Press Enter to start. Press Esc/Ctrl+C to quit.\n")
	case pageServer:
		b.WriteString(fmt.Sprintf("Server access  %s\n", mutedStyle.Render("[a] auth: "+string(m.cfg.Server.AuthMethod))))
		b.WriteString(m.renderInputs())
	case pageDomains:
		b.WriteString("Domains\n")
		b.WriteString(m.renderInputs())
	case pageAdmin:
		b.WriteString("First admin\n")
		b.WriteString(m.renderInputs())
	case pageMarketplaces:
		b.WriteString(fmt.Sprintf("Marketplaces  [w] WB: %v  [y] YM: %v  [o] Ozon: %v\n", m.cfg.Marketplaces.WB.Enabled, m.cfg.Marketplaces.YM.Enabled, m.cfg.Marketplaces.Ozon.Enabled))
		b.WriteString(m.renderInputs())
	case pageReview:
		m.saveInputs()
		b.WriteString("Review\n\n")
		b.WriteString(MaskedSummary(m.cfg) + "\n\n")
		b.WriteString("Press Enter to install. Press [u] to toggle auto-updates. Press Esc/Ctrl+C to quit.\n")
	case pageConsent:
		b.WriteString("Подтвердите перед установкой:\n")
		b.WriteString("  • Я имею право переопубликовывать отзывы со своих карточек товаров.\n")
		b.WriteString("  • Я ознакомлен(а) с обязанностями оператора персональных данных (152-ФЗ)\n")
		b.WriteString("    и размещу политику конфиденциальности\n")
		b.WriteString("    (шаблон: docs/legal/privacy-policy-template.ru.md).\n\n")
		b.WriteString("Нажмите Y для подтверждения · Q для выхода.\n")
	case pageInstalling:
		b.WriteString("Installing...\n\n")
		b.WriteString(renderEvents(m.events))
	case pageDone:
		if m.err != "" {
			b.WriteString(errStyle.Render("Installation failed") + "\n\n")
			b.WriteString(renderEvents(m.events))
			b.WriteString("\n" + errStyle.Render(m.err) + "\n")
		} else {
			b.WriteString(okStyle.Render("Installation complete") + "\n\n")
			b.WriteString("Admin: " + m.cfg.AdminURL() + "\n")
			b.WriteString("Widget base: " + m.cfg.WidgetBaseURL() + "\n")
			b.WriteString("Health: " + m.cfg.HealthURL() + "\n\n")
			b.WriteString("Next: open Admin -> Встраивание and copy the embed snippet.\n")
		}
		b.WriteString("\nPress Enter to exit.\n")
	}
	if len(m.inputs) > 0 {
		b.WriteString("\n" + mutedStyle.Render("Tab/Shift+Tab: move  Enter: next  Esc: quit") + "\n")
	}
	return b.String()
}

func (m model) renderInputs() string {
	var b strings.Builder
	for i, input := range m.inputs {
		prefix := "  "
		if i == m.focus {
			prefix = "> "
		}
		b.WriteString(prefix + input.Placeholder + "\n")
		b.WriteString("  " + input.View() + "\n\n")
	}
	return b.String()
}

func renderEvents(events []StepEvent) string {
	var b strings.Builder
	for _, event := range events {
		marker := "…"
		style := mutedStyle
		switch event.Status {
		case StepOK:
			marker = "✓"
			style = okStyle
		case StepFailed:
			marker = "✗"
			style = errStyle
		}
		line := fmt.Sprintf("%s %s", marker, event.Name)
		if event.Message != "" {
			line += ": " + event.Message
		}
		b.WriteString(style.Render(line) + "\n")
	}
	return b.String()
}
