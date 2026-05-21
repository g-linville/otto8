//go:build darwin || windows

package ui

import (
	"context"
	_ "embed"
	"errors"
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

type viewMode string

const (
	viewStatus viewMode = "status"
	viewURL    viewMode = "url"
	viewAgents viewMode = "agents"
	viewRun    viewMode = "run"
)

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
	urlEntry      *widget.Entry
	agentsBox     *fyne.Container
	progressBox   *fyne.Container
	messagesBox   *fyne.Container
	refreshButton *widget.Button
	runButton     *widget.Button
	cancelButton  *widget.Button
	body          *fyne.Container
	footer        *fyne.Container
	scroller      *container.Scroll

	mode           viewMode
	snapshot       setupstate.Snapshot
	selectedAgents map[string]bool
	setupURL       string
	setupCancel    context.CancelFunc
	running        bool
}

func newController(cfg Config) *controller {
	if cfg.CLIPath == "" {
		cfg.CLIPath = cli.DefaultPath()
	}
	if cfg.Client.Path == "" {
		cfg.Client.Path = cfg.CLIPath
	}
	return &controller{
		cfg:            cfg,
		mode:           viewStatus,
		selectedAgents: map[string]bool{},
	}
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
	c.urlEntry = widget.NewEntry()
	c.urlEntry.SetPlaceHolder("https://your-obot.example.com")
	c.messagesBox = container.NewVBox()
	c.agentsBox = container.NewVBox(helperRow(theme.InfoIcon(), "Detecting supported agents...", ""))
	c.progressBox = container.NewVBox(helperRow(theme.InfoIcon(), "Setup has not started", ""))

	c.refreshButton = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), c.load)
	c.runButton = widget.NewButtonWithIcon("Run Setup", theme.MediaPlayIcon(), c.startSetupFlow)
	c.cancelButton = widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), c.cancelSetup)
	c.cancelButton.Disable()

	c.statusPanelBG = panelBackground(statusPanelColor(setupstate.FirstRun), panelBorderColor)
	c.body = container.NewVBox(c.statusSection(), c.messagesBox)
	scrollContent := container.NewBorder(nil, nil, nil, rightGutter(), c.body)
	c.scroller = container.NewVScroll(scrollContent)
	c.footer = container.NewHBox(layout.NewSpacer(), c.refreshButton, c.runButton)

	go c.load()

	return container.NewBorder(
		container.NewPadded(titleHeader()),
		container.NewPadded(c.footer),
		nil,
		nil,
		container.NewPadded(c.scroller),
	)
}

func titleHeader() fyne.CanvasObject {
	iconResource := fyne.NewStaticResource("obot-icon-blue.svg", obotIconSVG)
	icon := canvas.NewImageFromResource(iconResource)
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(40, 40))

	title := widget.NewLabelWithStyle("Obot Setup", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	return container.NewHBox(icon, title)
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

func (c *controller) statusSection() fyne.CanvasObject {
	statusContent := container.NewBorder(
		nil,
		nil,
		container.NewPadded(c.statusIcon),
		nil,
		container.NewVBox(c.statusLabel, c.detailLabel),
	)
	return panel(c.statusPanelBG, statusContent)
}

func (c *controller) cliSection() fyne.CanvasObject {
	return section("CLI", panelSectionColor, widget.NewForm(
		widget.NewFormItem("Path", c.cliPathValue),
		widget.NewFormItem("Version", c.versionValue),
		widget.NewFormItem("Default URL", c.urlValue),
		widget.NewFormItem("Token", c.tokenValue),
	))
}

func (c *controller) agentsSection() fyne.CanvasObject {
	return section("Detected Agents", panelSectionColor, c.agentsBox)
}

func (c *controller) urlSetupSection() fyne.CanvasObject {
	content := container.NewBorder(
		nil,
		nil,
		container.NewPadded(c.statusIcon),
		nil,
		container.NewVBox(
			c.statusLabel,
			c.detailLabel,
			widget.NewForm(widget.NewFormItem("URL", c.urlEntry)),
		),
	)
	return panel(c.statusPanelBG, content)
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
		c.runButton.Disable()
		c.cancelButton.Disable()
		c.statusIcon.SetResource(theme.InfoIcon())
		c.statusLabel.SetText("Checking Obot CLI...")
		c.detailLabel.SetText("Loading local setup status and supported agent detection.")
		c.messagesBox.RemoveAll()
		c.messagesBox.Refresh()
		if c.mode == viewStatus {
			c.setBody(c.statusSection(), c.messagesBox)
			c.setFooter(layout.NewSpacer(), c.refreshButton, c.runButton)
		}
	})
}

func (c *controller) render(snapshot setupstate.Snapshot) {
	fyne.Do(func() {
		c.snapshot = snapshot
		if strings.TrimSpace(c.urlEntry.Text) == "" && snapshot.Status.DefaultURL != "" {
			c.urlEntry.SetText(snapshot.Status.DefaultURL)
		}
		c.statusIcon.SetResource(statusIcon(snapshot.Kind))
		c.statusPanelBG.FillColor = statusPanelColor(snapshot.Kind)
		c.statusPanelBG.Refresh()
		c.statusLabel.SetText(statusTitle(snapshot.Kind))
		c.detailLabel.SetText(statusDetail(snapshot))
		c.cliPathValue.SetText(snapshot.CLIPath)
		c.versionValue.SetText(versionText(snapshot))
		c.urlValue.SetText(urlText(snapshot))
		c.tokenValue.SetText(tokenText(snapshot))
		if c.mode == viewStatus {
			c.renderStatusAgents(snapshot)
			c.renderMessages(snapshot)
			c.setBody(
				c.statusSection(),
				c.cliSection(),
				c.agentsSection(),
				c.messagesBox,
			)
		}
		c.updateButtons()
	})
}

func (c *controller) setBody(objects ...fyne.CanvasObject) {
	c.body.RemoveAll()
	for _, object := range objects {
		c.body.Add(object)
	}
	c.body.Refresh()
}

func (c *controller) setFooter(objects ...fyne.CanvasObject) {
	c.footer.RemoveAll()
	for _, object := range objects {
		c.footer.Add(object)
	}
	c.footer.Refresh()
}

func (c *controller) renderStatusAgents(snapshot setupstate.Snapshot) {
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
		c.agentsBox.Add(agentStatusRow(agent))
	}
	c.agentsBox.Refresh()
}

func (c *controller) renderAgentChoices(snapshot setupstate.Snapshot) {
	c.agentsBox.RemoveAll()
	if snapshot.AgentsErr != nil {
		c.agentsBox.Add(helperRow(theme.WarningIcon(), "Agent detection failed", "Go back and try again, or run setup without local agent integrations."))
		c.agentsBox.Refresh()
		return
	}
	if len(snapshot.Agents) == 0 {
		c.agentsBox.Add(helperRow(theme.InfoIcon(), "No supported agents detected", "Setup can continue without installing local agent integrations."))
		c.agentsBox.Refresh()
		return
	}
	for i, agent := range snapshot.Agents {
		if i > 0 {
			c.agentsBox.Add(agentSeparator())
		}
		c.agentsBox.Add(c.agentCheckboxRow(agent))
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

func (c *controller) updateButtons() {
	if c.running {
		c.cancelButton.Enable()
		return
	}
	c.refreshButton.Enable()
	c.cancelButton.Disable()
	c.runButton.SetText(runButtonText(c.snapshot.Kind))
	if c.mode == viewStatus {
		c.setFooter(layout.NewSpacer(), c.refreshButton, c.runButton)
		if c.snapshot.Kind == setupstate.MissingCLI || c.snapshot.Kind == setupstate.UnsupportedCLI {
			c.runButton.Disable()
			return
		}
		c.runButton.Enable()
	}
}

func (c *controller) startSetupFlow() {
	c.mode = viewURL
	c.setupURL = strings.TrimSpace(c.snapshot.Status.DefaultURL)
	c.urlEntry.SetText(c.setupURL)
	c.renderURLStep()
	c.scrollToTop()
}

func (c *controller) scrollToTop() {
	if c.scroller != nil {
		c.scroller.ScrollToTop()
	}
}

func (c *controller) renderURLStep() {
	c.mode = viewURL
	c.statusLabel.SetText("Set Obot URL")
	c.detailLabel.SetText("Enter the Obot server URL to use for this setup run.")
	c.statusIcon.SetResource(theme.InfoIcon())
	c.statusPanelBG.FillColor = panelInfoColor
	c.statusPanelBG.Refresh()
	c.setBody(c.urlSetupSection())

	backButton := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), c.showStatusView)
	nextButton := widget.NewButtonWithIcon("Continue", theme.NavigateNextIcon(), c.continueFromURL)
	c.setFooter(layout.NewSpacer(), backButton, nextButton)
}

func (c *controller) continueFromURL() {
	url := strings.TrimSpace(c.urlEntry.Text)
	if url == "" {
		c.setBody(
			c.urlSetupSection(),
			messageCard(theme.WarningIcon(), "Enter the Obot server URL before continuing."),
		)
		return
	}
	c.setupURL = url
	c.loadAgentsForSetup()
}

func (c *controller) loadAgentsForSetup() {
	c.mode = viewAgents
	c.statusLabel.SetText("Looking for local agents")
	c.detailLabel.SetText("Checking this machine for supported local agent integrations.")
	c.statusIcon.SetResource(theme.InfoIcon())
	c.statusPanelBG.FillColor = panelInfoColor
	c.statusPanelBG.Refresh()
	c.agentsBox.RemoveAll()
	c.agentsBox.Add(helperRow(theme.InfoIcon(), "Detecting supported agents...", ""))
	c.agentsBox.Refresh()
	c.setBody(c.statusSection(), c.agentsSection())
	c.setFooter(layout.NewSpacer())

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		result, err := c.cfg.Client.DetectAgents(ctx)
		fyne.Do(func() {
			c.snapshot.AgentsErr = err
			if err == nil {
				c.snapshot.Agents = result.Agents
				c.resetSelectedAgents(result.Agents)
			} else {
				c.snapshot.Agents = nil
				c.selectedAgents = map[string]bool{}
			}
			c.renderAgentStep()
		})
	}()
}

func (c *controller) renderAgentStep() {
	c.mode = viewAgents
	c.statusLabel.SetText("Select local agents")
	c.detailLabel.SetText("Choose which detected local agents should receive the Obot bootstrap integration.")
	c.statusIcon.SetResource(theme.InfoIcon())
	c.statusPanelBG.FillColor = panelInfoColor
	c.statusPanelBG.Refresh()
	c.renderAgentChoices(c.snapshot)
	c.setBody(c.statusSection(), c.agentsSection())

	backButton := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), c.renderURLStep)
	startButton := widget.NewButtonWithIcon("Start Setup", theme.MediaPlayIcon(), c.runSetup)
	c.setFooter(layout.NewSpacer(), backButton, startButton)
}

func (c *controller) showStatusView() {
	c.mode = viewStatus
	c.render(c.snapshot)
}

func (c *controller) runSetup() {
	url := strings.TrimSpace(c.urlEntry.Text)
	if url == "" {
		c.renderURLStep()
		return
	}

	agentIDs := c.selectedAgentIDs()
	ctx, cancel := context.WithCancel(context.Background())
	c.setupCancel = cancel
	c.running = true
	c.mode = viewRun
	c.resetProgress()
	c.setProgress(theme.InfoIcon(), "Starting setup", "Obot will open browser login if the server requires authentication.")
	c.statusLabel.SetText("Running setup")
	c.detailLabel.SetText("Keep this window open while Obot configures the CLI and selected local agents.")
	c.statusIcon.SetResource(theme.InfoIcon())
	c.statusPanelBG.FillColor = panelInfoColor
	c.statusPanelBG.Refresh()
	c.setBody(c.statusSection(), section("Progress", panelSectionColor, c.progressBox))
	c.setFooter(layout.NewSpacer(), c.cancelButton)
	c.updateButtons()

	go func() {
		hadErrorEvent := false
		client := c.cfg.Client
		err := client.RunSetup(ctx, cli.SetupOptions{
			URL:      url,
			AgentIDs: agentIDs,
		}, func(event cli.SetupProgressEvent) {
			if event.Type == cli.SetupProgressError {
				hadErrorEvent = true
			}
			c.renderSetupEvent(event)
		})

		fyne.Do(func() {
			c.running = false
			c.setupCancel = nil
		})

		switch {
		case errors.Is(err, context.Canceled):
			c.setProgress(theme.WarningIcon(), "Setup canceled", "The running setup process was stopped.")
		case err != nil && !hadErrorEvent:
			c.setProgress(theme.ErrorIcon(), "Setup failed", err.Error())
		case err == nil:
			c.setProgress(theme.ConfirmIcon(), "Setup complete", "Obot CLI and selected local agents are configured.")
		}
		fyne.Do(c.renderDoneFooter)
	}()
}

func (c *controller) renderDoneFooter() {
	doneButton := widget.NewButtonWithIcon("Back to Status", theme.NavigateBackIcon(), func() {
		c.mode = viewStatus
		go c.load()
	})
	c.setFooter(layout.NewSpacer(), doneButton)
}

func (c *controller) cancelSetup() {
	if c.setupCancel != nil {
		c.setupCancel()
	}
}

func (c *controller) selectedAgentIDs() []string {
	var ids []string
	for _, agent := range c.snapshot.Agents {
		if agent.State == "present" && c.selectedAgents[agent.ID] {
			ids = append(ids, agent.ID)
		}
	}
	return ids
}

func (c *controller) resetSelectedAgents(agents []cli.Agent) {
	c.selectedAgents = map[string]bool{}
	for _, agent := range agents {
		c.selectedAgents[agent.ID] = agent.State == "present"
	}
}

func (c *controller) resetProgress() {
	fyne.Do(func() {
		c.progressBox.RemoveAll()
		c.progressBox.Refresh()
	})
}

func (c *controller) renderSetupEvent(event cli.SetupProgressEvent) {
	switch event.Type {
	case cli.SetupProgressAuthStarted:
		c.setProgress(theme.InfoIcon(), "Login started", setupEventURLDetail(event))
	case cli.SetupProgressAuthCompleted:
		c.setProgress(theme.ConfirmIcon(), "Login completed", setupEventURLDetail(event))
	case cli.SetupProgressConfigSaved:
		c.setProgress(theme.ConfirmIcon(), "CLI configuration saved", setupEventURLDetail(event))
	case cli.SetupProgressAgentInstalled:
		c.setProgress(theme.ConfirmIcon(), "Installed in "+event.DisplayName, strings.TrimSpace(event.Message))
	case cli.SetupProgressComplete:
		c.setProgress(theme.ConfirmIcon(), "Setup complete", setupEventURLDetail(event))
	case cli.SetupProgressError:
		c.setProgress(theme.ErrorIcon(), "Setup error", cli.SetupErrorDisplayMessage(event))
	default:
		c.setProgress(theme.InfoIcon(), displayState(event.Type), strings.TrimSpace(event.Message))
	}
}

func (c *controller) setProgress(icon fyne.Resource, title, detail string) {
	fyne.Do(func() {
		c.progressBox.RemoveAll()
		c.progressBox.Add(helperRow(icon, title, detail))
		c.progressBox.Refresh()
	})
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

func agentStatusRow(agent cli.Agent) fyne.CanvasObject {
	title := agent.DisplayName
	if state := displayState(agent.State); state != "" {
		title += " - " + state
	}
	return helperRow(agentIcon(agent.State), title, agent.Reason)
}

func (c *controller) agentCheckboxRow(agent cli.Agent) fyne.CanvasObject {
	title := agent.DisplayName
	if state := displayState(agent.State); state != "" {
		title += " - " + state
	}
	if _, ok := c.selectedAgents[agent.ID]; !ok {
		c.selectedAgents[agent.ID] = agent.State == "present"
	}
	check := widget.NewCheck(title, func(checked bool) {
		c.selectedAgents[agent.ID] = checked
	})
	check.SetChecked(c.selectedAgents[agent.ID] && agent.State == "present")
	if agent.State != "present" {
		check.Disable()
	}
	if agent.Reason == "" {
		return container.NewBorder(nil, nil, widget.NewIcon(agentIcon(agent.State)), nil, check)
	}

	detailLabel := widget.NewLabel(agent.Reason)
	detailLabel.Wrapping = fyne.TextWrapWord
	return container.NewBorder(nil, nil, widget.NewIcon(agentIcon(agent.State)), nil, container.NewVBox(check, detailLabel))
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

func runButtonText(kind setupstate.Kind) string {
	switch kind {
	case setupstate.AlreadyConfigured:
		return "Rerun Setup"
	case setupstate.NeedsLoginRepair:
		return "Repair Login"
	default:
		return "Run Setup"
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

func setupEventURLDetail(event cli.SetupProgressEvent) string {
	if strings.TrimSpace(event.URL) == "" {
		return strings.TrimSpace(event.Message)
	}
	if strings.TrimSpace(event.Message) == "" {
		return event.URL
	}
	return event.URL + "\n" + strings.TrimSpace(event.Message)
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
