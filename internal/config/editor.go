package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gumieri/nenyactl/internal/jsonc"
	"github.com/gumieri/nenyactl/internal/tui"
	"github.com/tailscale/hujson"
)

type configScreen int

const (
	screenSections configScreen = iota
	screenKeys
	screenEdit
	screenAgents
	// screenConfirmDelete gates a destructive operation (agent or entry
	// removal) behind an explicit y/n (NCTL-44).
	screenConfirmDelete
	// screenReview shows the full pending change set before anything is
	// written; save only happens from here (NCTL-44).
	screenReview
	// screenProviders lists configured providers with their key status and
	// records set/rotate ops (NCTL-45).
	screenProviders
	// screenProviderEdit is the masked key input for one provider (NCTL-45).
	screenProviderEdit
)

// confirmKind distinguishes what a pending confirmation would remove.
type confirmKind int

const (
	confirmDeleteAgent confirmKind = iota
	confirmDeleteEntry
	confirmDiscardAll
)

type sectionInfo struct {
	name    string
	isAgent bool
	// isProviders marks the section that opens the providers key-status
	// screen (NCTL-45): the raw JSON view cannot show whether a key is
	// wired, so the section opens the dedicated screen instead. Unlike
	// agents, it is never synthesized — the screen exists only when the
	// effective document has a providers section.
	isProviders bool
}

type configEntry struct {
	Key   string
	Value *hujson.Value
}

type agentEntry struct {
	Name     string
	Strategy string
	Models   []string
}

type configModel struct {
	screen        configScreen
	config        *hujson.Value
	sections      []sectionInfo
	cursor        int
	entries       []configEntry
	editKey       string
	editInput     textinput.Model
	activeSection string
	// activeScalar is true when the active section is a top-level scalar, so
	// its single entry is keyed by the section name and the dotted key would
	// otherwise double it.
	activeScalar bool

	// changes records edited top-level entries as dotted key -> raw JSON
	// value. It is the only output of the editor: every change is applied by
	// the caller through nenya's single writer (nenya config set), so nenyactl
	// never writes a config file itself.
	changes map[string]string

	// originalValues snapshots pre-edit values by dotted key so single-entry
	// revert ('u') can restore them (NCTL-46).
	originalValues map[string]string

	// agents section state
	agents             []agentEntry
	agentsModeAuto     bool
	autoAgentsOriginal bool
	agentsDirty        bool
	agentCursor        int

	sectionsView  viewport.Model
	keysView      viewport.Model
	agentsView    viewport.Model
	providersView viewport.Model
	width, height int
	helpModel     help.Model

	// confirm state: which screen to return to and what would be removed.
	confirmKind  confirmKind
	confirmIndex int
	// confirmFrom is the screen the confirmation was armed from, so n/esc
	// returns the user to their editing context.
	confirmFrom configScreen
	// reviewFrom is the screen the save was requested from, so esc returns
	// the user to their editing context.
	reviewFrom configScreen
	// validationMsg is an inline error on the providers/provider-edit
	// screens (empty key, etc.); cleared on navigation (NCTL-45).
	validationMsg string

	// Provider-key state (NCTL-45). providerRows drives the providers
	// screen; keyChanges holds typed (masked) values until the caller
	// applies them through nenya's single writer — the editor never writes
	// secrets itself.
	providerRows    []ProviderKeyStatus
	providerCursor  int
	keyInput        textinput.Model
	editingProvider string
	keyChanges      map[string]string

	saved bool
	quit  bool
}

// ProviderKeyStatus reports one provider's key presence for the providers
// screen: set/unset and where the key lives — never the value (NCTL-45).
// Name is the RAW config key; display sites run it through
// sanitizeConfigName, and ops recorded against it (KeyChanges) keep the raw
// name so the key lands under the provider nenya actually has configured.
type ProviderKeyStatus struct {
	Name   string
	KeySet bool
	// Source is where the key lives: "secrets dir" (single-writer managed,
	// from the local *.json shim), "config" (a config-level api_key), "env
	// ref"/"file ref" (defensive — describe redacts refs, so this only
	// appears against raw documents), or "" when unset.
	Source string
}

// KeyOp is one provider-key change made on the providers screen. The caller
// applies it through nenya's single writer — the same path as
// `nenyactl secret set --provider <name>` (NCTL-45).
type KeyOp struct {
	Provider string
	// Value is the new key. Rotate is the same op with a different value.
	Value string
}

// ProviderKeys returns the pending provider-key changes in stable order.
func (r *EditorResult) ProviderKeys() []KeyOp {
	names := make([]string, 0, len(r.KeyChanges))
	for name := range r.KeyChanges {
		names = append(names, name)
	}
	sort.Strings(names)
	ops := make([]KeyOp, 0, len(names))
	for _, name := range names {
		ops = append(ops, KeyOp{Provider: name, Value: r.KeyChanges[name]})
	}
	return ops
}

func newConfigModel(cfg *hujson.Value, effective []byte, keyStatus []ProviderKeyStatus) configModel {
	sections := jsonc.TopLevelKeys(cfg)

	// The effective config from describe already contains an "agents" section
	// once agents are configured; only add the synthetic one when it is absent
	// so the editor never renders two agents rows.
	hasAgents := false
	for _, key := range sections {
		if key == "agents" {
			hasAgents = true
			break
		}
	}
	extra := 0
	if !hasAgents {
		extra = 1
	}

	m := configModel{
		screen:         screenSections,
		config:         cfg,
		sections:       make([]sectionInfo, len(sections)+extra),
		cursor:         0,
		editInput:      textinput.New(),
		sectionsView:   viewport.New(0, 0),
		keysView:       viewport.New(0, 0),
		agentsView:     viewport.New(0, 0),
		providersView:  viewport.New(0, 0),
		helpModel:      tui.NewHelpModel(),
		changes:        make(map[string]string),
		originalValues: make(map[string]string),
	}
	m.editInput.CharLimit = 256
	m.editInput.Width = 50

	// The provider-key input is masked: keys are credentials and must not
	// land in terminal scrollback (NCTL-45). The limit is generous — provider
	// keys are typically < 512 bytes, but silent truncation of a longer key
	// would record a broken credential.
	m.keyInput = textinput.New()
	m.keyInput.Placeholder = "sk-..."
	m.keyInput.EchoMode = textinput.EchoPassword
	m.keyInput.EchoCharacter = '•'
	m.keyInput.CharLimit = 2048
	m.keyInput.Width = 50
	m.keyChanges = make(map[string]string)
	m.providerRows = buildProviderRows(cfg, keyStatus)

	// Fill sections array. An existing "agents" section opens the agents
	// editor (not the raw JSON view, where object values cannot be edited); a
	// synthetic one is appended when the effective config has none. The
	// providers section opens the key-status screen only when it is an
	// object; arrays/scalars fall back to the generic JSON view.
	for i, key := range sections {
		isProvidersObj := false
		if key == "providers" {
			if pv, ok := jsonc.GetField(cfg, key); ok {
				_, isProvidersObj = pv.Value.(*hujson.Object)
			}
		}
		m.sections[i] = sectionInfo{
			name:        key,
			isAgent:     key == "agents",
			isProviders: isProvidersObj,
		}
	}
	// Add the synthetic agents section at the end when the effective config
	// does not already carry one.
	if !hasAgents {
		m.sections[len(sections)] = sectionInfo{
			name:    "agents",
			isAgent: true,
		}
	}

	// Agents come from the effective document (nenya describe), never from a
	// drop-in path: the editor must not read or merge config files itself.
	m.agents, m.agentsModeAuto = agentsFromEffective(effective)
	m.autoAgentsOriginal = m.agentsModeAuto

	// Seed the viewport content so the first render is populated before any
	// WindowSizeMsg arrives (the scroll helpers early-return at height 0).
	m.updateSectionsContent()
	m.updateAgentsContent()
	m.updateProvidersContent()

	return m
}

// resetCursor moves the section cursor to the top and refreshes the view.
func (m *configModel) resetCursor() {
	m.cursor = 0
	m.scrollSections()
	m.updateSectionsContent()
}

// effectiveAgents is the subset of the effective config the agents editor
// needs. It is parsed from `nenya describe --json`, the authoritative merged
// document, so the editor never reads a config file or a drop-in directly.
type effectiveAgents struct {
	Discovery struct {
		AutoAgents *bool `json:"auto_agents"`
	} `json:"discovery"`
	Agents map[string]struct {
		Strategy string   `json:"strategy"`
		Models   []string `json:"models"`
	} `json:"agents"`
}

// agentsFromEffective extracts the configured agents and the auto-agents flag
// from the effective config JSON.
func agentsFromEffective(effective []byte) ([]agentEntry, bool) {
	var doc effectiveAgents
	if err := json.Unmarshal(effective, &doc); err != nil {
		return nil, false
	}

	names := make([]string, 0, len(doc.Agents))
	for name := range doc.Agents {
		names = append(names, name)
	}
	sort.Strings(names)

	auto := false
	if doc.Discovery.AutoAgents != nil {
		auto = *doc.Discovery.AutoAgents
	}

	if len(names) == 0 {
		return nil, auto
	}

	agents := make([]agentEntry, 0, len(names))
	for _, name := range names {
		a := doc.Agents[name]
		strategy := a.Strategy
		if strategy == "" {
			strategy = "fallback"
		}
		agents = append(agents, agentEntry{Name: name, Strategy: strategy, Models: a.Models})
	}

	return agents, auto
}

func (m configModel) Init() tea.Cmd { return nil }

// helpKeyMap returns the key map for the active screen. The sections and agents
// screens handle "s" (save); the keys/edit screens do not, so they must not
// advertise it.
func (m configModel) helpKeyMap() tui.KeyMap {
	switch m.screen {
	case screenSections:
		return tui.SectionsKeyMap
	case screenAgents:
		return tui.AgentsKeyMap
	case screenKeys:
		return tui.KeysKeyMap
	case screenEdit:
		return tui.ValueEditKeyMap
	case screenConfirmDelete:
		return tui.ConfirmKeyMap
	case screenReview:
		return tui.ReviewKeyMap
	case screenProviders:
		return tui.ProvidersKeyMap
	case screenProviderEdit:
		return tui.ProviderEditKeyMap
	default:
		return tui.SectionsKeyMap
	}
}

func (m *configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		h := msg.Height - 6
		if h < 8 {
			h = 8
		}
		m.sectionsView.Width = msg.Width - 6
		m.sectionsView.Height = h
		m.keysView.Width = msg.Width - 6
		m.keysView.Height = h
		m.agentsView.Width = msg.Width - 6
		m.agentsView.Height = h
		m.providersView.Width = msg.Width - 6
		m.providersView.Height = h

	case tea.KeyMsg:
		switch m.screen {
		case screenSections:
			return m.updateSections(msg)
		case screenKeys:
			return m.updateKeys(msg)
		case screenEdit:
			return m.updateEdit(msg)
		case screenAgents:
			return m.updateAgents(msg)
		case screenProviders:
			return m.updateProviders(msg)
		case screenProviderEdit:
			// updateProviderEdit fully handles KeyMsgs (commands + input
			// forwarding); only non-KeyMsg messages reach the input update
			// below.
			return m.updateProviderEdit(msg)
		case screenConfirmDelete:
			return m.updateConfirmDelete(msg)
		case screenReview:
			return m.updateReview(msg)
		}
	}

	switch m.screen {
	case screenEdit:
		m.editInput, _ = m.editInput.Update(msg)
	case screenProviderEdit:
		m.keyInput, _ = m.keyInput.Update(msg)
	}

	return m, nil
}

func (m *configModel) updateSections(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.quit = true
		return m, tea.Quit
	case "enter":
		if m.cursor < len(m.sections) {
			section := m.sections[m.cursor]
			if section.isAgent {
				m.screen = screenAgents
				m.agentCursor = 0
				m.updateAgentsContent()
			} else if section.isProviders {
				// NCTL-45: the providers section opens the key-status screen
				// instead of the raw JSON view, which cannot show whether a
				// key is actually wired (describe redacts it either way).
				m.screen = screenProviders
				m.providerCursor = 0
				m.validationMsg = ""
				m.updateProvidersContent()
				m.scrollProviders()
			} else {
				m.loadSection(section.name)
				m.screen = screenKeys
			}
		}
		return m, nil
	case "s":
		m.beginReview()
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollSections()
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.sections)-1 {
			m.cursor++
			m.scrollSections()
		}
		return m, nil
	}
	return m, nil
}

// beginReview moves to the save-review screen, remembering where the user
// came from (NCTL-44: save shows what will change before writing).
func (m *configModel) beginReview() {
	m.reviewFrom = m.screen
	m.screen = screenReview
}

func (m *configModel) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.screen = screenSections
		m.cursor = 0
		m.scrollSections()
		return m, nil
	case "enter":
		if m.cursor < len(m.entries) {
			m.startEdit(m.cursor)
			m.screen = screenEdit
		}
		return m, nil
	case "u":
		// NCTL-46: revert the entry under the cursor to its pre-edit value.
		if m.cursor >= 0 && m.cursor < len(m.entries) {
			m.revertEntry(m.cursor)
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.scrollKeys()
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m.scrollKeys()
		}
		return m, nil
	}
	return m, nil
}

// originals snapshots the pre-edit value of every entry the first time it is
// modified in this session (dotted key -> raw JSON literal). Revert restores
// from here (NCTL-46).
func (m *configModel) originals() map[string]string {
	if m.originalValues == nil {
		m.originalValues = make(map[string]string)
	}
	return m.originalValues
}

// snapshotEntry records the entry's original value before its first edit.
func (m *configModel) snapshotEntry(dottedKey string, value string) {
	if _, exists := m.originals()[dottedKey]; !exists {
		m.originals()[dottedKey] = value
	}
}

// revertEntry restores the entry at idx to its pre-edit value, dropping the
// pending change. Entries never edited in this session are left alone
// (NCTL-46).
func (m *configModel) revertEntry(idx int) {
	if idx < 0 || idx >= len(m.entries) {
		return
	}
	entry := m.entries[idx]
	dottedKey := m.activeSection + "." + entry.Key
	if m.activeScalar && entry.Key == m.activeSection {
		dottedKey = entry.Key
	}
	original, has := m.originals()[dottedKey]
	if !has {
		return
	}
	entry.Value.Value = parseLiteralValue(original)
	delete(m.changes, dottedKey)
	delete(m.originals(), dottedKey)
}

// updateAgentsGlobal handles quit/save/section-exit keys on the agents screen.
func (m *configModel) updateAgentsGlobal(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit, true
	case "esc":
		m.screen = screenSections
		m.cursor = 0
		m.scrollSections()
		return m, nil, true
	case "s":
		m.beginReview()
		return m, nil, true
	}
	return m, nil, false
}

// updateAgentsAction handles the enter/a/d/space keys that edit the agent list.
func (m *configModel) updateAgentsAction(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case " ":
		m.agentsModeAuto = !m.agentsModeAuto
		m.updateAgentsContent()
		return m, nil, true
	case "enter":
		m.startEditAgent()
		return m, nil, true
	case "a":
		m.agents = append(m.agents, agentEntry{Name: "new-agent", Strategy: "fallback"})
		m.agentCursor = len(m.agents) - 1
		m.startEditAgent()
		return m, nil, true
	case "d":
		if m.agentCursor >= 0 && m.agentCursor < len(m.agents) {
			// NCTL-44: gate the destructive delete behind an explicit
			// confirmation instead of removing the agent silently.
			m.confirmKind = confirmDeleteAgent
			m.confirmIndex = m.agentCursor
			m.confirmFrom = screenAgents
			m.screen = screenConfirmDelete
		}
		return m, nil, true
	}
	return m, nil, false
}

// updateProviders handles navigation and edit entry on the providers screen
// (NCTL-45).
func (m *configModel) updateProviders(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.screen = screenSections
		m.cursor = 0
		m.scrollSections()
		return m, nil
	case "enter":
		if m.providerCursor >= 0 && m.providerCursor < len(m.providerRows) {
			raw := m.providerRows[m.providerCursor].Name
			if raw == "" || sanitizeConfigName(raw) != raw {
				// Refuse, never re-key: the op must land under the exact
				// config name, and an empty or control-bearing name is
				// hostile input.
				m.validationMsg = "provider name is empty or contains control characters — fix it in the config file first"
				return m, nil
			}
			m.editingProvider = raw
			m.keyInput.SetValue("")
			m.validationMsg = ""
			m.screen = screenProviderEdit
			return m, m.keyInput.Focus()
		}
		return m, nil
	case "up", "k":
		if m.providerCursor > 0 {
			m.providerCursor--
			m.validationMsg = ""
			m.updateProvidersContent()
			m.scrollProviders()
		}
		return m, nil
	case "down", "j":
		if m.providerCursor < len(m.providerRows)-1 {
			m.providerCursor++
			m.validationMsg = ""
			m.updateProvidersContent()
			m.scrollProviders()
		}
		return m, nil
	}
	return m, nil
}

// updateProviderEdit resolves the masked key input: enter records the
// set/rotate op (applied later by the caller through nenya's single writer),
// esc discards it (NCTL-45).
func (m *configModel) updateProviderEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.editingProvider = ""
		m.validationMsg = ""
		m.keyInput.Blur()
		m.updateProvidersContent()
		m.screen = screenProviders
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.keyInput.Value())
		if value == "" {
			m.validationMsg = "key is empty — type a key, or esc to cancel"
			return m, nil
		}
		m.keyChanges[m.editingProvider] = value
		m.editingProvider = ""
		m.validationMsg = ""
		m.keyInput.Blur()
		m.updateProvidersContent()
		m.screen = screenProviders
		return m, nil
	}
	// Unhandled keys (typing, backspace, arrows) reach the masked input here:
	// the outer Update switch returns on handled KeyMsgs first, so without
	// this forward the input would never receive keystrokes.
	m.keyInput, _ = m.keyInput.Update(msg)
	return m, nil
}

// updateConfirmDelete resolves a pending destructive-operation confirmation:
// y performs it, n/esc returns to the originating screen unchanged.
func (m *configModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		switch m.confirmKind {
		case confirmDeleteAgent:
			idx := m.confirmIndex
			if idx >= 0 && idx < len(m.agents) {
				m.agents = append(m.agents[:idx], m.agents[idx+1:]...)
				m.agentsDirty = true
				if m.agentCursor >= len(m.agents) && m.agentCursor > 0 {
					m.agentCursor--
				}
				m.updateAgentsContent()
			}
			m.screen = screenAgents
		case confirmDeleteEntry:
			m.screen = screenKeys
		case confirmDiscardAll:
			m.discardAll()
			m.screen = m.reviewFrom
		}
		m.confirmKind = 0
		m.confirmIndex = -1
		return m, nil
	case "n", "esc":
		m.confirmKind = 0
		m.confirmIndex = -1
		m.screen = m.confirmFrom
		return m, nil
	}
	return m, nil
}

// updateReview resolves the save-review screen: s/enter writes, esc returns
// to the screen the save was requested from (NCTL-44).
func (m *configModel) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s", "enter":
		m.saved = true
		return m, tea.Quit
	case "n", "esc":
		m.screen = m.reviewFrom
		return m, nil
	case "D":
		// NCTL-46: explicit discard-all with confirmation. 'D' only arms the
		// gate; y performs it.
		m.confirmKind = confirmDiscardAll
		m.confirmFrom = screenReview
		m.screen = screenConfirmDelete
		return m, nil
	}
	return m, nil
}

// discardAll drops every pending change and restores the pre-edit values
// (NCTL-46).
func (m *configModel) discardAll() {
	for dottedKey, original := range m.originalValues {
		section, key, found := strings.Cut(dottedKey, ".")
		if !found {
			section, key = dottedKey, dottedKey
		}
		if section != m.activeSection {
			continue
		}
		for i := range m.entries {
			if m.entries[i].Key == key {
				m.entries[i].Value.Value = parseLiteralValue(original)
				break
			}
		}
	}
	m.changes = make(map[string]string)
	m.originalValues = make(map[string]string)
	m.keyChanges = make(map[string]string)
	m.agents = nil
	m.agentsDirty = false
	m.resetCursor()
}

// updateAgentsNavigation handles the up/down agent cursor movement.
func (m *configModel) updateAgentsNavigation(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "up", "k":
		if m.agentCursor > 0 {
			m.agentCursor--
			m.scrollAgents()
		}
		return m, nil, true
	case "down", "j":
		if m.agentCursor < len(m.agents) {
			m.agentCursor++
			m.scrollAgents()
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m *configModel) updateAgents(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if model, cmd, handled := m.updateAgentsGlobal(key); handled {
		return model, cmd
	}
	if model, cmd, handled := m.updateAgentsAction(key); handled {
		return model, cmd
	}
	model, cmd, _ := m.updateAgentsNavigation(key)
	return model, cmd
}

func (m *configModel) loadSection(sectionName string) {
	m.activeSection = sectionName
	m.activeScalar = false
	field, ok := jsonc.GetField(m.config, sectionName)
	if !ok {
		m.entries = nil
		m.cursor = 0
		m.updateKeysContent()
		return
	}

	obj, ok := field.Value.(*hujson.Object)
	if !ok {
		m.activeScalar = true
		m.entries = []configEntry{{Key: sectionName, Value: field}}
		m.cursor = 0
		m.scrollKeys()
		return
	}

	m.entries = make([]configEntry, len(obj.Members))
	for i := range obj.Members {
		m.entries[i] = configEntry{
			Key:   obj.Members[i].Name.Value.(hujson.Literal).String(),
			Value: &obj.Members[i].Value,
		}
	}
	m.cursor = 0
	m.scrollKeys()
}

func (m *configModel) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "esc":
		m.screen = screenKeys
		return m, nil
	case "enter":
		m.applyEdit()
		m.screen = screenKeys
		return m, nil
	}
	// Forward typed characters (and editing keys) to the value input; the
	// early returns above prevent the outer Update from doing it.
	m.editInput, _ = m.editInput.Update(msg)
	return m, nil
}

func (m *configModel) startEdit(idx int) {
	if idx < 0 || idx >= len(m.entries) {
		return
	}
	entry := m.entries[idx]
	m.editKey = entry.Key
	m.editInput.SetValue(jsonc.FieldValueString(entry.Value))
	m.editInput.CursorEnd()
	m.editInput.Focus()
}

// applyAgentNameEdit applies the edit input to the selected agent's name: an
// empty value removes the agent. It reports whether the edit was an agent-name
// edit (the caller then stops processing).
func (m *configModel) applyAgentNameEdit() bool {
	if m.editKey != "agent_name" || m.agentCursor < 0 || m.agentCursor >= len(m.agents) {
		return false
	}
	name := m.editInput.Value()
	if name == "" {
		// NCTL-44: an empty name used to delete the agent silently. Route
		// through the delete confirmation instead; the actual removal
		// happens in updateConfirmDelete.
		m.confirmKind = confirmDeleteAgent
		m.confirmIndex = m.agentCursor
		m.confirmFrom = screenAgents
		m.screen = screenConfirmDelete
		return true
	}
	m.agents[m.agentCursor].Name = name
	m.agentsDirty = true
	m.screen = screenAgents
	m.updateAgentsContent()
	return true
}

func (m *configModel) applyEdit() {
	// Handle agent name editing
	if m.applyAgentNameEdit() {
		return
	}

	if len(m.entries) == 0 || m.cursor < 0 || m.cursor >= len(m.entries) {
		return
	}

	entry := m.entries[m.cursor]
	raw := m.editInput.Value()

	if _, ok := entry.Value.Value.(*hujson.Object); ok {
		return
	}
	if _, ok := entry.Value.Value.(*hujson.Array); ok {
		return
	}

	// NCTL-46: snapshot the pre-edit value BEFORE the assignment replaces it,
	// so 'u' can revert this entry to what it was when the session found it.
	dottedKey := m.activeSection + "." + entry.Key
	if m.activeScalar && entry.Key == m.activeSection {
		dottedKey = entry.Key
	}
	m.snapshotEntry(dottedKey, m.entryRawString(entry))

	entry.Value.Value = parseLiteralValue(raw)
	m.changes[dottedKey] = string(parseLiteralValue(raw))
}

func (m *configModel) scrollSections() {
	visible := m.sectionsView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	if m.cursor < half {
		m.sectionsView.YOffset = 0
	} else if m.cursor > len(m.sections)-1-half {
		m.sectionsView.YOffset = len(m.sections) - visible
		if m.sectionsView.YOffset < 0 {
			m.sectionsView.YOffset = 0
		}
	} else {
		m.sectionsView.YOffset = m.cursor - half
	}
	m.updateSectionsContent()
}

func (m *configModel) scrollKeys() {
	visible := m.keysView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	if m.cursor < half {
		m.keysView.YOffset = 0
	} else if m.cursor > len(m.entries)-1-half {
		m.keysView.YOffset = len(m.entries) - visible
		if m.keysView.YOffset < 0 {
			m.keysView.YOffset = 0
		}
	} else {
		m.keysView.YOffset = m.cursor - half
	}
	m.updateKeysContent()
}

func (m *configModel) scrollAgents() {
	visible := m.agentsView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	total := len(m.agents) + 1
	if m.agentCursor < half {
		m.agentsView.YOffset = 0
	} else if m.agentCursor > total-1-half {
		m.agentsView.YOffset = total - visible
		if m.agentsView.YOffset < 0 {
			m.agentsView.YOffset = 0
		}
	} else {
		m.agentsView.YOffset = m.agentCursor - half
	}
	m.updateAgentsContent()
}

func (m configModel) View() string {
	theme := tui.Current()

	if m.quit || m.saved {
		return ""
	}

	var content string
	switch m.screen {
	case screenSections:
		content = m.viewSections()
	case screenKeys:
		content = m.viewKeys()
	case screenEdit:
		content = m.viewEdit()
	case screenAgents:
		content = m.viewAgents()
	case screenConfirmDelete:
		content = m.viewConfirmDelete(theme)
	case screenReview:
		content = m.viewReview(theme)
	case screenProviders:
		content = m.viewProviders(theme)
	case screenProviderEdit:
		content = m.viewProviderEdit(theme)
	}

	helpView := m.helpModel.View(m.helpKeyMap())

	out := lipgloss.JoinVertical(lipgloss.Top,
		content,
		"",
		helpView,
	)

	return theme.App.Render(out)
}

func (m configModel) viewSections() string {
	theme := tui.Current()

	title := theme.Title.Render("Edit Configuration")
	var fileInfo string
	if len(m.sections) > 0 {
		fileInfo = theme.Dimmed.Render("Enter to expand section · s to save · q to quit")
	}

	body := lipgloss.JoinVertical(lipgloss.Top, title, "", fileInfo, "", m.sectionsView.View())

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m configModel) viewKeys() string {
	theme := tui.Current()

	title := theme.Title.Render(fmt.Sprintf("Edit: %s", m.activeSection))
	subtitle := theme.Dimmed.Render("Enter to save value · Esc to go back")

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		subtitle,
		"",
		m.keysView.View(),
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

func (m configModel) viewEdit() string {
	theme := tui.Current()

	title := theme.Title.Render(fmt.Sprintf("Edit: %s", m.editKey))

	inputView := m.editInput.View()
	if m.editInput.Focused() {
		inputView = theme.InputFocused.Render(inputView)
	} else {
		inputView = theme.InputBlurred.Render(inputView)
	}

	// Agent-name edits leave m.entries from the previous section, so only show a
	// "Current:" line when the cursor actually addresses an entry.
	var currentVal string
	if m.editKey != "agent_name" && m.cursor >= 0 && m.cursor < len(m.entries) {
		currentVal = theme.Dimmed.Render("Current: " + jsonc.FieldValueString(m.entries[m.cursor].Value))
	}
	body := lipgloss.JoinVertical(lipgloss.Top, title, "", currentVal, "", inputView, "", theme.Dimmed.Render("Enter to save · Esc to cancel"))

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

// viewConfirmDelete renders the destructive-operation gate: what would be
// removed and the y/n choice (NCTL-44).
func (m configModel) viewConfirmDelete(theme tui.Theme) string {
	subject := "this entry"
	switch m.confirmKind {
	case confirmDeleteAgent:
		if m.confirmIndex >= 0 && m.confirmIndex < len(m.agents) {
			subject = fmt.Sprintf("agent %q", m.agents[m.confirmIndex].Name)
		} else {
			subject = "this agent"
		}
	case confirmDiscardAll:
		subject = "ALL pending changes"
	}
	prompt := fmt.Sprintf("Delete %s? This cannot be undone within the editor.\n\ny/enter — delete\nn/esc  — cancel", subject)
	if m.confirmKind == confirmDiscardAll {
		prompt = fmt.Sprintf("Discard %s? Edits in the current section are reverted and every pending change (config keys and provider keys) is dropped.\n\ny/enter — discard\nn/esc  — cancel", subject)
	}
	body := theme.Body.Render(prompt)
	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}

// viewReview renders the pending change set before anything is written
// (NCTL-44): every dotted key with its new value, plus the agents note.
func (m configModel) viewReview(theme tui.Theme) string {
	var b strings.Builder
	b.WriteString(theme.Title.Render("Review changes"))
	b.WriteString("\n\n")

	if len(m.changes) == 0 && !m.agentsDirty && len(m.keyChanges) == 0 {
		b.WriteString(theme.Body.Render("No changes to save."))
	}

	if m.agentsDirty {
		b.WriteString(theme.Body.Render("• agents: rewritten per the edited list\n"))
	}
	for _, k := range sortedChangeKeys(m.changes) {
		fmt.Fprintf(&b, "%s %s = %s\n", theme.Dimmed.Render("•"), theme.Body.Render(k), m.changes[k])
	}

	// Provider-key ops are listed by name only — the typed key is never
	// rendered (NCTL-45).
	keyNames := make([]string, 0, len(m.keyChanges))
	for name := range m.keyChanges {
		keyNames = append(keyNames, name)
	}
	sort.Strings(keyNames)
	for _, name := range keyNames {
		fmt.Fprintf(&b, "%s provider key %s: set/rotate via nenya secret writer\n",
			theme.Dimmed.Render("•"), theme.Body.Render(name))
	}

	b.WriteString("\n")
	b.WriteString(theme.Body.Render("s/enter — save and apply\nD      — discard all changes (asks)\nn/esc  — back to editing"))
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

// viewProviders renders the provider key-status screen (NCTL-45): set/unset
// and the key source — never the value. Pending key changes are flagged so
// the user sees what will be applied on save.
func (m configModel) viewProviders(theme tui.Theme) string {
	title := theme.Title.Render("Provider API Keys")
	subtitle := theme.Dimmed.Render("enter — set or rotate the selected provider's key · esc — back")

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		subtitle,
		"",
		m.providersView.View(),
	)

	if m.validationMsg != "" {
		body = lipgloss.JoinVertical(lipgloss.Top,
			body,
			"",
			theme.Error.Render(m.validationMsg),
		)
	}

	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}

// updateProvidersContent refreshes the providers rows viewport (NCTL-45).
func (m *configModel) updateProvidersContent() {
	m.providersView.SetContent(m.renderProviderRows())
}

// scrollProviders keeps the provider cursor visible (mirrors scrollKeys) and
// refreshes the viewport content, matching its sibling scroll helpers.
func (m *configModel) scrollProviders() {
	visible := m.providersView.Height
	if visible <= 0 {
		return
	}
	half := visible / 2
	if m.providerCursor < half {
		m.providersView.YOffset = 0
	} else if m.providerCursor > len(m.providerRows)-1-half {
		m.providersView.YOffset = len(m.providerRows) - visible
		if m.providersView.YOffset < 0 {
			m.providersView.YOffset = 0
		}
	} else {
		m.providersView.YOffset = m.providerCursor - half
	}
	m.updateProvidersContent()
}

// renderProviderRows renders the provider rows only — the viewport content.
// Status shows set/unset and the source; key values are never rendered.
func (m configModel) renderProviderRows() string {
	theme := tui.Current()
	var b strings.Builder
	if len(m.providerRows) == 0 {
		b.WriteString(theme.Body.Render("No providers configured."))
	}
	for i, row := range m.providerRows {
		prefix := "  "
		if i == m.providerCursor {
			prefix = "● "
		}
		status := theme.Error.Render("no key set")
		if row.KeySet {
			status = theme.Success.Render("key set (" + row.Source + ")")
		}
		display := sanitizeConfigName(row.Name)
		if display == "" {
			display = "(unprintable name)"
		}
		line := fmt.Sprintf("%s%-22s %s", prefix, display, status)
		if _, pending := m.keyChanges[row.Name]; pending {
			line += "  " + theme.Dimmed.Render("[pending change]")
		}
		b.WriteString(theme.Body.Render(line))
		b.WriteString("\n")
	}
	return b.String()
}

// viewProviderEdit renders the masked key input for one provider (NCTL-45).
// nenya enforces key strength at apply time (a weak key is refused by the
// single writer after the editor exits), and the key transits to nenya as a
// command argument (visible in the process list while it runs) — both
// documented here so the user knows the boundaries before typing.
func (m configModel) viewProviderEdit(theme tui.Theme) string {
	var b strings.Builder
	b.WriteString(theme.Title.Render("Set key: " + m.editingProvider))
	b.WriteString("\n\n")
	b.WriteString(theme.Body.Render("Applied through nenya's single writer on save; nenya refuses weak keys.\nThe key is masked while typing, never displayed, and passed to nenya\nas a command argument (visible in the process list while it runs).\n\n"))
	b.WriteString(theme.InputFocused.Render(m.keyInput.View()))
	if m.validationMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(theme.Error.Render(m.validationMsg))
	}
	b.WriteString("\n\n")
	b.WriteString(theme.Dimmed.Render("enter — record the key\nesc — cancel"))
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m configModel) viewAgents() string {
	theme := tui.Current()

	var autoLabel string
	var autoStyle lipgloss.Style
	if m.agentsModeAuto {
		autoLabel = "ON"
		autoStyle = theme.Success
	} else {
		autoLabel = "OFF"
		autoStyle = theme.Error
	}

	title := theme.Title.Render("Configure Agents")
	autoLine := fmt.Sprintf("Auto-agents: %s  (Space to toggle, Enter to configure manually)",
		autoStyle.Bold(true).Render(autoLabel))

	body := lipgloss.JoinVertical(lipgloss.Top,
		title,
		"",
		autoLine,
		"",
		m.agentsView.View(),
	)

	baseStyle := lipgloss.NewStyle().
		Width(m.width).
		BorderStyle(theme.Border).
		BorderForeground(theme.BorderColor.GetForeground()).
		Padding(1, 2)

	return baseStyle.Render(body)
}

// sectionTypeHint renders the dimmed hint shown after a section name: the
// agent count for the agents pseudo-section, or the value type/summary.
func (m configModel) sectionTypeHint(section sectionInfo, theme tui.Theme) string {
	if section.isAgent {
		return theme.Dimmed.Render(fmt.Sprintf("  %d agent(s)", len(m.agents)))
	}
	field, ok := jsonc.GetField(m.config, section.name)
	if !ok {
		return ""
	}
	switch field.Value.(type) {
	case *hujson.Object:
		return theme.Dimmed.Render("  {object}")
	case *hujson.Array:
		return theme.Dimmed.Render("  [array]")
	default:
		val := jsonc.FieldValueString(field)
		if len(val) > 50 {
			val = val[:47] + "..."
		}
		return theme.Dimmed.Render("  " + val)
	}
}

func (m configModel) renderSections() string {
	theme := tui.Current()
	var lines []string

	for i, section := range m.sections {
		var prefix string
		style := theme.Body
		if i == m.cursor {
			prefix = "● "
			style = theme.Highlight
		} else {
			prefix = "  "
		}

		typeHint := m.sectionTypeHint(section, theme)

		nameStyle := theme.Accent
		if i == m.cursor {
			nameStyle = nameStyle.Bold(true)
		}

		name := section.name
		if section.isAgent {
			nameStyle = theme.Success
		}

		line := fmt.Sprintf("%s%s%s", style.Render(prefix), nameStyle.Render(name), typeHint)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m configModel) renderAgents() string {
	theme := tui.Current()
	var lines []string

	for i, a := range m.agents {
		var prefix string
		style := theme.Body
		if i == m.agentCursor {
			prefix = "● "
			style = theme.Highlight
		} else {
			prefix = "  "
		}

		modelsStr := strings.Join(a.Models, ", ")
		if modelsStr == "" {
			modelsStr = "(no models)"
		}

		nameStyle := theme.Accent
		if i == m.agentCursor {
			nameStyle = nameStyle.Bold(true)
		}

		name := nameStyle.Render(a.Name)
		strategy := theme.Dimmed.Render(a.Strategy)
		modelsLine := theme.Dimmed.Render(fmt.Sprintf("  %d models: %s", len(a.Models), trunc(modelsStr, 50)))

		line := fmt.Sprintf("%s%s  %s\n    %s",
			style.Render(prefix),
			name,
			strategy,
			modelsLine,
		)
		lines = append(lines, line)
	}

	addPrefix := "  "
	if m.agentCursor == len(m.agents) {
		addPrefix = "● "
	}
	addLabel := theme.Accent.Render(addPrefix + "+ Add new agent")
	lines = append(lines, addLabel)

	return strings.Join(lines, "\n")
}

func (m *configModel) updateAgentsContent() {
	m.agentsView.SetContent(m.renderAgents())
}

func (m *configModel) updateSectionsContent() {
	m.sectionsView.SetContent(m.renderSections())
}

func (m *configModel) updateKeysContent() {
	m.keysView.SetContent(m.renderKeys())
}

// EditorResult is the set of changes the user made. It carries no file paths:
// the caller applies each change through nenya's single writer, which owns the
// target file and the merge.
type EditorResult struct {
	// Changes maps a dotted config key to its raw JSON value.
	Changes map[string]string
	// KeyChanges maps a provider name to a new API key typed on the
	// providers screen. The caller applies each through nenya's single
	// writer; the editor never writes secrets itself (NCTL-45).
	KeyChanges map[string]string
}

// buildProviderRows assembles the providers screen rows from the effective
// document's providers section, merged with secrets-file key presence passed
// by the caller (NENYA-166 references and config-level api_key literals are
// detected from the document itself; secrets.json names come in via
// keyStatus). Values are never read — only presence.
func buildProviderRows(cfg *hujson.Value, keyStatus []ProviderKeyStatus) []ProviderKeyStatus {
	secretSet := make(map[string]ProviderKeyStatus, len(keyStatus))
	for _, s := range keyStatus {
		secretSet[s.Name] = s
	}

	providersValue, ok := jsonc.GetField(cfg, "providers")
	if !ok {
		return nil
	}
	obj, ok := providersValue.Value.(*hujson.Object)
	if !ok {
		return nil
	}

	rows := make([]ProviderKeyStatus, 0, len(obj.Members))
	type rowSort struct {
		row       ProviderKeyStatus
		sanitized string
	}
	sorted := make([]rowSort, 0, len(obj.Members))
	for i := range obj.Members {
		raw := jsonc.MemberName(&obj.Members[i])
		display := sanitizeConfigName(raw)
		if display == "" {
			// Nothing safely renderable: keep the row so the operator can
			// see the provider exists; renderProviderRows shows a
			// placeholder and edit entry refuses control-bearing names.
			display = "(unprintable name)"
		}
		row := ProviderKeyStatus{Name: raw}
		if st, ok := secretSet[raw]; ok && st.KeySet {
			row.KeySet = true
			row.Source = st.Source
		}
		applyConfigKeySource(&row, &obj.Members[i].Value)
		sorted = append(sorted, rowSort{row: row, sanitized: display})
	}
	// Deterministic order by the display name, raw name as tiebreaker.
	sort.Slice(sorted, func(x, y int) bool {
		if sorted[x].sanitized != sorted[y].sanitized {
			return sorted[x].sanitized < sorted[y].sanitized
		}
		return sorted[x].row.Name < sorted[y].row.Name
	})
	for _, s := range sorted {
		rows = append(rows, s.row)
	}
	return rows
}

// applyConfigKeySource marks the row as set when the provider entry carries a
// config-level api_key, classifying env:/file: references (NCTL-45). Presence
// is what the screen reports, never the value. The api_key member is read as
// a hujson.Literal directly: string literals decode via Literal.String()
// (raw literal text keeps JSON quotes), and non-string kinds are not
// credentials.
//
// Two contract realities shape this: `nenya describe` redacts every set
// api_key to the [REDACTED] sentinel (so a literal ref is never observable in
// the effective document — the ref branch is defensive), and nenya's
// ResolveProviders uses ONLY secrets.provider_keys for the runtime key — so a
// secrets.json entry (row.Source already set) always wins the label over a
// config-level key.
func applyConfigKeySource(row *ProviderKeyStatus, provider *hujson.Value) {
	apiKey, ok := jsonc.GetNestedField(provider, []string{"api_key"})
	if !ok {
		return
	}
	// Only a string literal carries a credential; numbers, bools, null,
	// objects and arrays are not keys.
	lit, isLit := apiKey.Value.(hujson.Literal)
	if !isLit || lit.Kind() != '"' {
		return
	}
	// Decode the literal: the raw text keeps JSON quotes, which would defeat
	// the sentinel/ref comparisons below.
	v := lit.String()
	if v == "" {
		return
	}
	row.KeySet = true
	if row.Source != "" {
		// A secrets.json key is nenya's runtime source of truth; keep its
		// label even when a config-level api_key also exists.
		return
	}
	switch {
	case v == redactedSentinel:
		// Same label as the default arm today (nenya's runtime key comes from
		// secrets either way); kept separate so a future sentinel-specific
		// source ("describe redacted") doesn't have to untangle the default.
		row.Source = "config"
	case strings.HasPrefix(v, "env:") || strings.HasPrefix(v, "file:"):
		row.Source = strings.SplitN(v, ":", 2)[0] + " ref"
	default:
		row.Source = "config"
	}
}

// redactedSentinel mirrors nenya describe's credential redaction marker
// (netutil.RedactedUserinfoSentinel): a set api_key shows up as exactly this
// value in the effective document.
const redactedSentinel = "[REDACTED]"

// sanitizeConfigName strips terminal control sequences and codepoints from
// config-derived names so a hostile config cannot inject output into the TUI:
// ESC sequences (CSI 0x40–0x7E finals, OSC bodies through BEL/ST), C0
// controls, DEL, and C1 codepoints U+0080–U+009F. Valid UTF-8 passes through
// untouched (runes are decoded, so multi-byte sequences — whose continuation
// bytes live in 0x80–0xBF — are never corrupted). Names only ever feed the
// display and the --provider argument; a name the sanitizer changes is never
// silently re-keyed (updateProviders warns and refuses).
func sanitizeConfigName(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); {
		c := name[i]
		if c == '\x1b' {
			if i+1 < len(name) && name[i+1] == '[' {
				i += skipCSI(name, i)
				continue
			}
			if i+1 < len(name) && name[i+1] == ']' {
				i += skipOSC(name, i)
				continue
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(name[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte: drop it (jsonc documents are valid UTF-8;
			// this is hostile-input defense).
			i++
			continue
		}
		if r <= 0x1f || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			i += size
			continue
		}
		b.WriteString(name[i : i+size])
		i += size
	}
	return b.String()
}

// skipCSI swallows a CSI sequence (ESC [ … final byte 0x40–0x7E) starting at
// the ESC and returns the number of bytes consumed.
func skipCSI(name string, esc int) int {
	for i := esc + 2; i < len(name); i++ {
		if name[i] >= 0x40 && name[i] <= 0x7e {
			return i - esc + 1
		}
	}
	return len(name) - esc
}

// skipOSC swallows an OSC body (ESC ] … BEL or ESC-\) starting at the ESC and
// returns the number of bytes consumed.
func skipOSC(name string, esc int) int {
	for i := esc + 2; i < len(name); i++ {
		switch {
		case name[i] == '\x07':
			return i - esc + 1
		case name[i] == '\x1b' && i+1 < len(name) && name[i+1] == '\\':
			return i - esc + 2
		}
	}
	return len(name) - esc
}

func (m *configModel) startEditAgent() {
	if m.agentCursor >= 0 && m.agentCursor < len(m.agents) {
		// Agent editing uses the same value input as config edits; applyEdit
		// reads editInput, so seed and focus that (not editName).
		m.screen = screenEdit
		m.editKey = "agent_name" // Marker for agent editing
		m.editInput.SetValue(m.agents[m.agentCursor].Name)
		m.editInput.CursorEnd()
		m.editInput.Focus()
	}
}

func (m configModel) renderKeys() string {
	theme := tui.Current()
	var lines []string

	for i, entry := range m.entries {
		var prefix string
		style := theme.Body
		if i == m.cursor {
			prefix = "● "
			style = theme.Highlight
		} else {
			prefix = "  "
		}

		keyStyle := theme.Accent
		if i == m.cursor {
			keyStyle = keyStyle.Bold(true)
		}

		val := jsonc.FieldValueString(entry.Value)
		if len(val) > 60 {
			val = val[:57] + "..."
		}

		line := fmt.Sprintf("%s%s  %s", style.Render(prefix), keyStyle.Render(entry.Key), theme.Dimmed.Render(val))
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// RunConfigEditor runs the interactive editor over the effective config. The
// caller supplies the authoritative effective document (from
// `nenya describe --json`), so the editor never reads or merges config files
// itself, plus the secrets-file provider-key status for the providers screen
// (names and presence only — never values). Provider-key changes come back in
// EditorResult.KeyChanges for the caller to apply through nenya's single
// writer.
func RunConfigEditor(effective []byte, keyStatus []ProviderKeyStatus) (*EditorResult, bool, error) {
	cfg, err := jsonc.ParseDoc(effective)
	if err != nil {
		return nil, false, err
	}

	m := newConfigModel(cfg, effective, keyStatus)
	m.resetCursor()

	p := tea.NewProgram(&m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, false, err
	}

	tm, ok := result.(*configModel)
	if !ok {
		return nil, false, nil
	}

	if tm.quit || !tm.saved {
		return nil, false, nil
	}

	res, err := tm.result()
	if err != nil {
		return nil, false, err
	}
	return res, true, nil
}

// result assembles the changes the user made. It is separate from the TUI run
// loop so the assembly (not just the interactive path) is unit-testable.
func (m *configModel) result() (*EditorResult, error) {
	changes := make(map[string]string, len(m.changes)+1)
	for k, v := range m.changes {
		changes[k] = v
	}
	if m.agentsDirty {
		value, err := agentsValue(m.agents)
		if err != nil {
			return nil, err
		}
		changes["agents"] = value
	}
	if m.agentsModeAuto != m.autoAgentsOriginal {
		changes["discovery.auto_agents"] = strconv.FormatBool(m.agentsModeAuto)
	}
	keyChanges := make(map[string]string, len(m.keyChanges))
	for k, v := range m.keyChanges {
		keyChanges[k] = v
	}
	return &EditorResult{Changes: changes, KeyChanges: keyChanges}, nil
}

// ParseConfigValue renders a raw flag value as a JSON literal using the same
// rules as the TUI value editor: bare strings are quoted, true/false/null
// pass through (NCTL-48). Unlike the TUI, passthrough values (quoted strings,
// objects, arrays, numbers) are validated here — a malformed literal fails
// locally before any write instead of silently persisting as a string via
// nenya's lenient fallback. Bare strings are JSON-escaped (control characters
// and invalid UTF-8 included), so the result is always a valid JSON literal.
func ParseConfigValue(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return `""`, nil
	}
	l := strings.ToLower(trimmed)
	switch l {
	case "true", "false", "null":
		return l, nil
	}
	switch trimmed[0] {
	case '"', '{', '[':
		if !json.Valid([]byte(trimmed)) {
			return "", fmt.Errorf("value %q looks like JSON (starts with %q) but does not parse", raw, trimmed[0])
		}
		return trimmed, nil
	}
	if isJSONNumber(trimmed) {
		return trimmed, nil
	}
	// Bare string: JSON-escape it (handles control characters and invalid
	// UTF-8 that strconv.Quote would escape in non-JSON \x form).
	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return "", fmt.Errorf("value %q cannot be encoded as a JSON string: %w", raw, err)
	}
	return string(encoded), nil
}

// isJSONNumber reports whether s is a valid JSON number: strconv.ParseFloat
// accepts Inf/NaN, hex-float, underscore, and leading-zero forms that JSON
// rejects, so the parsed value is cross-checked with json.Valid (NCTL-48
// review).
func isJSONNumber(s string) bool {
	if !json.Valid([]byte(s)) {
		return false
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// sortedChangeKeys returns a change set's keys in stable order — shared by
// the TUI review screen and the --print-diff renderer.
func sortedChangeKeys(changes map[string]string) []string {
	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// RenderChanges renders a pending change set as plain sorted `key = value`
// lines — the non-interactive equivalent of the TUI review screen (NCTL-48).
// Values are the JSON literals that would be applied, exactly as recorded.
func RenderChanges(changes map[string]string) string {
	if len(changes) == 0 {
		return "No changes.\n"
	}
	var b strings.Builder
	for _, k := range sortedChangeKeys(changes) {
		fmt.Fprintf(&b, "%s = %s\n", k, changes[k])
	}
	return b.String()
}

func trunc(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func parseLiteralValue(raw string) hujson.Literal {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return hujson.Literal(`""`)
	}
	switch strings.ToLower(raw) {
	case "true":
		return hujson.Literal("true")
	case "false":
		return hujson.Literal("false")
	case "null":
		return hujson.Literal("null")
	}
	if raw[0] == '"' || raw[0] == '{' || raw[0] == '[' {
		return hujson.Literal(raw)
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return hujson.Literal(raw)
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return hujson.Literal(raw)
	}
	return hujson.Literal(fmt.Sprintf("%q", raw))
}

// agentsValue renders the agents list as the JSON value for the config's
// top-level "agents" key, suitable for `nenya config set agents <json>`. It
// rejects empty and duplicate names, which would otherwise collapse silently
// when marshaled into a map.
func agentsValue(agents []agentEntry) (string, error) {
	agentsMap := make(map[string]any)
	for _, a := range agents {
		if a.Name == "" {
			return "", fmt.Errorf("agent name cannot be empty")
		}
		if _, dup := agentsMap[a.Name]; dup {
			return "", fmt.Errorf("duplicate agent name %q", a.Name)
		}
		agentsMap[a.Name] = map[string]any{
			"strategy": a.Strategy,
			"models":   nonNilStrings(a.Models),
		}
	}
	data, err := json.Marshal(agentsMap)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// nonNilStrings returns a non-nil slice so an agent with no models marshals as
// "models": [] rather than null.
func nonNilStrings(models []string) []string {
	if models == nil {
		return []string{}
	}
	return models
}

// entryRawString renders the entry's current raw value as JSON text, for the
// pre-edit snapshot (NCTL-46).
func (m *configModel) entryRawString(entry configEntry) string {
	if lit, ok := entry.Value.Value.(hujson.Literal); ok {
		return lit.String()
	}
	raw, err := json.Marshal(entry.Value.Value)
	if err != nil {
		return ""
	}
	return string(raw)
}
