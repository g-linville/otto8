//go:build darwin || windows

package ui

import (
	"context"
	_ "embed"
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/obot-platform/obot/packaging/ObotSetup/internal/cli"
	"github.com/obot-platform/obot/packaging/ObotSetup/internal/setupstate"
)

//go:embed assets/obot-icon-blue.svg
var obotIconSVG []byte

type Config struct {
	AppVersion string
	CLIPath    string
	Client     cli.Client
}

func Run(cfg Config) {
	a := app.NewWithID("ai.obot.setup")
	w := a.NewWindow("Obot Setup")
	w.Resize(fyne.NewSize(720, 520))
	w.SetContent(newController(cfg).content())
	w.ShowAndRun()
}

type controller struct {
	cfg Config

	statusIcon    *widget.Icon
	statusPanelBG *canvas.Rectangle
	statusLabel   *widget.Label
	detailLabel   *widget.Label
	cliPathValue  *widget.Label
	urlValue      *widget.Label
	tokenValue    *widget.Label
	versionValue  *widget.Label
	agentsBox     *fyne.Container
	messagesBox   *fyne.Container
	refreshButton *widget.Button
}

func newController(cfg Config) *controller {
	if cfg.CLIPath == "" {
		cfg.CLIPath = cli.DefaultPath()
	}
	if cfg.Client.Path == "" {
		cfg.Client.Path = cfg.CLIPath
	}
	return &controller{cfg: cfg}
}

func (c *controller) content() fyne.CanvasObject {
	c.statusIcon = widget.NewIcon(theme.InfoIcon())
	c.statusLabel = widget.NewLabel("Checking Obot CLI...")
	c.statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	c.detailLabel = widget.NewLabel("Loading local setup status and supported agent detection.")
	c.detailLabel.Wrapping = fyne.TextWrapWord

	c.cliPathValue = valueLabel()
	c.urlValue = valueLabel()
	c.tokenValue = valueLabel()
	c.versionValue = valueLabel()
	c.messagesBox = container.NewVBox()
	c.agentsBox = container.NewVBox(helperRow(theme.InfoIcon(), "Detecting supported agents...", ""))

	c.refreshButton = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), c.load)

	c.statusPanelBG = panelBackground(statusPanelColor(setupstate.FirstRun), panelBorderColor)
	statusContent := container.NewBorder(
		nil,
		nil,
		container.NewPadded(c.statusIcon),
		nil,
		container.NewVBox(c.statusLabel, c.detailLabel),
	)
	statusSection := panel(c.statusPanelBG, statusContent)
	cliSection := section("CLI", panelSectionColor, widget.NewForm(
		widget.NewFormItem("Path", c.cliPathValue),
		widget.NewFormItem("Version", c.versionValue),
		widget.NewFormItem("Default URL", c.urlValue),
		widget.NewFormItem("Token", c.tokenValue),
	))
	agentsSection := section("Supported Agents", panelSectionColor, c.agentsBox)

	body := container.NewVBox(
		statusSection,
		cliSection,
		agentsSection,
		c.messagesBox,
	)
	scrollContent := container.NewBorder(nil, nil, nil, rightGutter(), body)
	footer := container.NewHBox(layout.NewSpacer(), c.refreshButton)

	go c.load()

	return container.NewBorder(
		container.NewPadded(titleHeader()),
		container.NewPadded(footer),
		nil,
		nil,
		container.NewPadded(container.NewVScroll(scrollContent)),
	)
}

func titleHeader() fyne.CanvasObject {
	iconResource := fyne.NewStaticResource("obot-icon-blue.svg", obotIconSVG)
	icon := canvas.NewImageFromResource(iconResource)
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(40, 40))

	title := widget.NewLabelWithStyle("Obot Setup", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel("Local CLI and agent setup")
	subtitle.TextStyle = fyne.TextStyle{Italic: true}

	return container.NewHBox(icon, container.NewVBox(title, subtitle))
}

func section(title string, fill color.Color, content fyne.CanvasObject) fyne.CanvasObject {
	heading := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return panel(panelBackground(fill, panelBorderColor), container.NewVBox(heading, content))
}

func panel(bg *canvas.Rectangle, content fyne.CanvasObject) fyne.CanvasObject {
	return container.NewStack(bg, container.NewPadded(content))
}

func panelBackground(fill, stroke color.Color) *canvas.Rectangle {
	bg := canvas.NewRectangle(fill)
	bg.StrokeColor = stroke
	bg.StrokeWidth = 1
	bg.CornerRadius = 6
	return bg
}

func rightGutter() fyne.CanvasObject {
	gutter := canvas.NewRectangle(color.Transparent)
	gutter.SetMinSize(fyne.NewSize(12, 1))
	return gutter
}

func valueLabel() *widget.Label {
	label := widget.NewLabel("")
	label.Wrapping = fyne.TextWrapWord
	return label
}

func (c *controller) load() {
	c.setLoading()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client := c.cfg.Client
	cliExists := client.Exists()

	var (
		status    cli.Status
		statusErr error
		agents    []cli.Agent
		agentsErr error
	)
	if cliExists {
		status, statusErr = client.Status(ctx)
		if statusErr == nil {
			result, err := client.DetectAgents(ctx)
			if err != nil {
				agentsErr = err
			} else {
				agents = result.Agents
			}
		}
	}

	snapshot := setupstate.Build(c.cfg.CLIPath, c.cfg.AppVersion, cliExists, status, statusErr, agents, agentsErr)
	c.render(snapshot)
}

func (c *controller) setLoading() {
	fyne.Do(func() {
		c.refreshButton.Disable()
		c.statusIcon.SetResource(theme.InfoIcon())
		c.statusLabel.SetText("Checking Obot CLI...")
		c.detailLabel.SetText("Loading local setup status and supported agent detection.")
		c.messagesBox.RemoveAll()
		c.messagesBox.Refresh()
	})
}

func (c *controller) render(snapshot setupstate.Snapshot) {
	fyne.Do(func() {
		c.refreshButton.Enable()
		c.statusIcon.SetResource(statusIcon(snapshot.Kind))
		c.statusPanelBG.FillColor = statusPanelColor(snapshot.Kind)
		c.statusPanelBG.Refresh()
		c.statusLabel.SetText(statusTitle(snapshot.Kind))
		c.detailLabel.SetText(statusDetail(snapshot))
		c.cliPathValue.SetText(snapshot.CLIPath)
		c.versionValue.SetText(versionText(snapshot))
		c.urlValue.SetText(urlText(snapshot))
		c.tokenValue.SetText(tokenText(snapshot))
		c.renderAgents(snapshot)
		c.renderMessages(snapshot)
	})
}

func (c *controller) renderAgents(snapshot setupstate.Snapshot) {
	c.agentsBox.RemoveAll()
	if snapshot.Kind == setupstate.MissingCLI || snapshot.StatusErr != nil {
		c.agentsBox.Add(helperRow(theme.InfoIcon(), "Agent detection unavailable", "Agent detection needs a supported Obot CLI."))
		c.agentsBox.Refresh()
		return
	}
	if snapshot.AgentsErr != nil {
		c.agentsBox.Add(helperRow(theme.WarningIcon(), "Agent detection failed", "Refresh to try again."))
		c.agentsBox.Refresh()
		return
	}
	if len(snapshot.Agents) == 0 {
		c.agentsBox.Add(helperRow(theme.InfoIcon(), "No supported agents detected", "The CLI did not report any local agent integrations."))
		c.agentsBox.Refresh()
		return
	}
	for i, agent := range snapshot.Agents {
		if i > 0 {
			c.agentsBox.Add(agentSeparator())
		}
		c.agentsBox.Add(agentRow(agent))
	}
	c.agentsBox.Refresh()
}

func (c *controller) renderMessages(snapshot setupstate.Snapshot) {
	c.messagesBox.RemoveAll()
	if warnings := warningsText(snapshot); warnings != "" {
		c.messagesBox.Add(messageCard(theme.WarningIcon(), warnings))
	}
	if err := errorText(snapshot); err != "" {
		c.messagesBox.Add(messageCard(theme.ErrorIcon(), err))
	}
	c.messagesBox.Refresh()
}

func helperRow(icon fyne.Resource, title, detail string) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	if detail == "" {
		return container.NewBorder(nil, nil, widget.NewIcon(icon), nil, titleLabel)
	}

	detailLabel := widget.NewLabel(detail)
	detailLabel.Wrapping = fyne.TextWrapWord
	return container.NewBorder(nil, nil, widget.NewIcon(icon), nil, container.NewVBox(titleLabel, detailLabel))
}

func agentRow(agent cli.Agent) fyne.CanvasObject {
	title := agent.DisplayName
	if state := displayState(agent.State); state != "" {
		title += " - " + state
	}
	return helperRow(agentIcon(agent.State), title, agent.Reason)
}

func agentSeparator() fyne.CanvasObject {
	line := canvas.NewRectangle(panelBorderColor)
	line.SetMinSize(fyne.NewSize(1, 1))
	return container.NewPadded(line)
}

func messageCard(icon fyne.Resource, message string) fyne.CanvasObject {
	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord
	return panel(panelBackground(panelWarningColor, panelWarningBorderColor), container.NewBorder(nil, nil, widget.NewIcon(icon), nil, label))
}

func statusIcon(kind setupstate.Kind) fyne.Resource {
	switch kind {
	case setupstate.AlreadyConfigured:
		return theme.ConfirmIcon()
	case setupstate.MissingCLI, setupstate.UnsupportedCLI, setupstate.NeedsLoginRepair:
		return theme.WarningIcon()
	default:
		return theme.InfoIcon()
	}
}

func agentIcon(state string) fyne.Resource {
	if state == "present" {
		return theme.ConfirmIcon()
	}
	return theme.InfoIcon()
}

func statusTitle(kind setupstate.Kind) string {
	switch kind {
	case setupstate.MissingCLI:
		return "Obot CLI is not installed"
	case setupstate.UnsupportedCLI:
		return "Installed Obot CLI is not supported"
	case setupstate.AlreadyConfigured:
		return "Obot is configured"
	case setupstate.NeedsLoginRepair:
		return "Obot needs login repair"
	default:
		return "Obot needs setup"
	}
}

func statusDetail(snapshot setupstate.Snapshot) string {
	switch snapshot.Kind {
	case setupstate.MissingCLI:
		return "The setup app expected to find obot at the installed macOS package location."
	case setupstate.UnsupportedCLI:
		return "Install a newer Obot CLI that supports non-interactive setup, JSON status, and JSON progress."
	case setupstate.AlreadyConfigured:
		return "The CLI has a configured Obot URL and a valid stored token."
	case setupstate.NeedsLoginRepair:
		return "A default Obot URL is configured, but the stored token is missing or invalid."
	default:
		return "Enter setup details in the next stage to configure the CLI and local agent integrations."
	}
}

func versionText(snapshot setupstate.Snapshot) string {
	if !snapshot.CLIExists || snapshot.Status.Version == "" {
		return "Unavailable"
	}
	return snapshot.Status.Version
}

func urlText(snapshot setupstate.Snapshot) string {
	if snapshot.Status.DefaultURL == "" {
		return "Not configured"
	}
	return snapshot.Status.DefaultURL
}

func tokenText(snapshot setupstate.Snapshot) string {
	if snapshot.Status.DefaultURL == "" {
		return "Not checked"
	}
	if snapshot.Status.TokenValid {
		return "Valid"
	}
	return "Missing or invalid"
}

func warningsText(snapshot setupstate.Snapshot) string {
	var warnings []string
	if snapshot.VersionMismatch {
		warnings = append(warnings, fmt.Sprintf("Warning: setup app version %s differs from CLI version %s.", snapshot.AppVersion, snapshot.Status.Version))
	}
	if snapshot.CLIExists && !snapshot.UsrLocalBinInPath {
		warnings = append(warnings, "Warning: /usr/local/bin is not present in this process PATH; users may need to add it to their shell PATH.")
	}
	if len(snapshot.MissingCapabilities) > 0 {
		warnings = append(warnings, "Missing CLI capabilities: "+strings.Join(snapshot.MissingCapabilities, ", ")+".")
	}
	return strings.Join(warnings, "\n")
}

func errorText(snapshot setupstate.Snapshot) string {
	switch {
	case snapshot.StatusErr != nil:
		return "Status error: " + snapshot.StatusErr.Error()
	case snapshot.AgentsErr != nil:
		return "Agent detection error: " + snapshot.AgentsErr.Error()
	default:
		return ""
	}
}

func displayState(state string) string {
	state = strings.TrimSpace(state)
	if state == "" {
		return ""
	}
	return strings.ToUpper(state[:1]) + state[1:]
}

var (
	panelBorderColor        = color.NRGBA{R: 0x38, G: 0x3d, B: 0x46, A: 0xff}
	panelSectionColor       = color.NRGBA{R: 0x22, G: 0x26, B: 0x2d, A: 0xff}
	panelInfoColor          = color.NRGBA{R: 0x1f, G: 0x27, B: 0x32, A: 0xff}
	panelSuccessColor       = color.NRGBA{R: 0x1d, G: 0x2b, B: 0x24, A: 0xff}
	panelWarningColor       = color.NRGBA{R: 0x31, G: 0x29, B: 0x1d, A: 0xff}
	panelWarningBorderColor = color.NRGBA{R: 0x6e, G: 0x58, B: 0x2b, A: 0xff}
)

func statusPanelColor(kind setupstate.Kind) color.Color {
	switch kind {
	case setupstate.AlreadyConfigured:
		return panelSuccessColor
	case setupstate.MissingCLI, setupstate.UnsupportedCLI, setupstate.NeedsLoginRepair:
		return panelWarningColor
	default:
		return panelInfoColor
	}
}
