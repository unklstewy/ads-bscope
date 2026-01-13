package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unklstewy/ads-bscope/pkg/config"
)

// configMenuModel represents the configuration menu state.
type configMenuModel struct {
	cfg         *config.Config // Working copy of configuration
	originalCfg *config.Config // Original config for revert
	configPath  string         // Path to config file

	// Navigation state
	inMainMenu      bool   // True if in main menu, false if in submenu
	currentSection  int    // Which section is selected in main menu
	currentField    int    // Which field in current submenu
	editing         bool   // Whether we're currently editing a field
	editBuffer      string // Buffer for text editing
	editingRegion   bool   // Whether we're editing region details
	regionEditField int    // Which region field is being edited (0=name, 1=lat, 2=lon, 3=radius)

	// Status
	dirty          bool   // Whether config has unsaved changes
	message        string // Status message to display
	messageIsError bool   // Whether message is an error

	// UI state
	width  int
	height int
}

// ConfigSection represents a section in the config menu.
type ConfigSection int

const (
	SectionGeneral ConfigSection = iota
	SectionObserver
	SectionRegions
	SectionTelescope
	SectionADSB
	SectionDatabase
	NumSections
)

func (s ConfigSection) String() string {
	switch s {
	case SectionGeneral:
		return "GENERAL"
	case SectionObserver:
		return "OBSERVER"
	case SectionRegions:
		return "COLLECTION REGIONS"
	case SectionTelescope:
		return "TELESCOPE"
	case SectionADSB:
		return "ADS-B"
	case SectionDatabase:
		return "DATABASE (Read-Only)"
	default:
		return "UNKNOWN"
	}
}

// newConfigMenuModel creates a new configuration menu.
func newConfigMenuModel(cfg *config.Config, configPath string) configMenuModel {
	// Deep copy config for working copy
	workingCfg := *cfg
	originalCfg := *cfg

	return configMenuModel{
		cfg:            &workingCfg,
		originalCfg:    &originalCfg,
		configPath:     configPath,
		inMainMenu:     true, // Start in main menu
		currentSection: 0,
		currentField:   0,
		dirty:          false,
	}
}

func (m configMenuModel) Init() tea.Cmd {
	return nil
}

func (m configMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// If editing a field, handle edit mode keys
		if m.editing {
			return m.handleEditMode(msg)
		}

		// Navigation mode keys
		switch msg.String() {
		case "esc":
			// If editing region, exit region edit mode
			if m.editingRegion {
				m.editingRegion = false
				m.regionEditField = 0
				m.message = ""
				return m, nil
			}
			// If in submenu, go back to main menu
			if !m.inMainMenu {
				m.inMainMenu = true
				m.currentField = 0
				m.message = ""
				return m, nil
			}
			// Otherwise exit config menu (will be handled by parent)
			return m, tea.Quit

		case "q":
			// Q always exits completely
			return m, tea.Quit

		case "up", "k":
			m.navigateUp()
			m.message = ""

		case "down", "j":
			m.navigateDown()
			m.message = ""

		case "enter":
			if m.inMainMenu {
				// Enter submenu
				m.inMainMenu = false
				m.currentField = 0
				m.message = ""
			} else if m.editingRegion {
				// Already in region edit mode - start editing the selected field
				m.startEditing()
			} else {
				// In submenu - check if it's a special action or edit
				if ConfigSection(m.currentSection) == SectionRegions {
					// Check if "Add New Region" is selected
					if m.currentField == len(m.cfg.ADSB.CollectionRegions) {
						m.addNewRegion()
						return m, nil
					}
					// Otherwise enter region edit mode
					m.editingRegion = true
					m.regionEditField = 0 // Start with name
					m.message = "Editing region. Use ↑/↓ to select field, ENTER to edit, ESC when done"
					return m, nil
				}
				// Start editing selected field
				m.startEditing()
			}

		case " ":
			// Space key - toggle boolean fields or regions
			if !m.inMainMenu && !m.editing {
				if ConfigSection(m.currentSection) == SectionRegions {
					// Toggle region enabled/disabled
					if m.currentField < len(m.cfg.ADSB.CollectionRegions) {
						m.toggleRegion()
					}
					return m, nil
				}
				// For other boolean fields, could add toggle logic here
			}

		case "x":
			// Delete region (only in regions submenu, use X to avoid conflict with D for defaults)
			if !m.inMainMenu && ConfigSection(m.currentSection) == SectionRegions {
				if m.currentField < len(m.cfg.ADSB.CollectionRegions) {
					m.deleteRegion()
				}
				return m, nil
			}

		case "d":
			// Restore defaults (only from main menu)
			if m.inMainMenu {
				m.restoreDefaults()
			}

		case "s":
			// Save configuration
			return m, m.saveConfig()

		case "r":
			// Reload from file
			return m, m.reloadConfig()

		case "o":
			// Create observer position from region (only if in regions submenu)
			if !m.inMainMenu && ConfigSection(m.currentSection) == SectionRegions {
				if m.currentField < len(m.cfg.ADSB.CollectionRegions) {
					m.createObserverFromRegion()
				}
				return m, nil
			}
		}
	}

	return m, nil
}

// handleEditMode handles keypresses while editing a field.
func (m configMenuModel) handleEditMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Cancel editing
		m.editing = false
		m.editBuffer = ""
		m.message = "Edit cancelled"

	case "enter":
		// Save field value
		if err := m.saveFieldValue(); err != nil {
			m.message = fmt.Sprintf("Error: %v", err)
			m.messageIsError = true
		} else {
			m.editing = false
			m.editBuffer = ""
			m.dirty = true
			m.message = "Field updated (not saved)"
			m.messageIsError = false
		}

	case "backspace":
		if len(m.editBuffer) > 0 {
			m.editBuffer = m.editBuffer[:len(m.editBuffer)-1]
		}

	default:
		// Add character to buffer
		if len(msg.String()) == 1 {
			m.editBuffer += msg.String()
		}
	}

	return m, nil
}

// navigateUp moves selection up.
func (m *configMenuModel) navigateUp() {
	if m.editingRegion {
		// Navigating region fields
		if m.regionEditField > 0 {
			m.regionEditField--
		} else {
			m.regionEditField = 3 // Wrap to radius
		}
	} else if m.inMainMenu {
		// In main menu, navigate sections
		if m.currentSection > 0 {
			m.currentSection--
		} else {
			m.currentSection = int(NumSections) - 1
		}
	} else {
		// In submenu, navigate fields
		if m.currentField > 0 {
			m.currentField--
		} else {
			// Wrap to last field
			m.currentField = m.getFieldCount(ConfigSection(m.currentSection)) - 1
		}
	}
}

// navigateDown moves selection down.
func (m *configMenuModel) navigateDown() {
	if m.editingRegion {
		// Navigating region fields
		if m.regionEditField < 3 {
			m.regionEditField++
		} else {
			m.regionEditField = 0 // Wrap to name
		}
	} else if m.inMainMenu {
		// In main menu, navigate sections
		m.currentSection = (m.currentSection + 1) % int(NumSections)
	} else {
		// In submenu, navigate fields
		fieldCount := m.getFieldCount(ConfigSection(m.currentSection))
		if m.currentField < fieldCount-1 {
			m.currentField++
		} else {
			// Wrap to first field
			m.currentField = 0
		}
	}
}

// getFieldCount returns the number of fields in a section.
func (m *configMenuModel) getFieldCount(section ConfigSection) int {
	switch section {
	case SectionGeneral:
		return 3 // port, update_interval, rate_limit
	case SectionObserver:
		return 5 // name, lat, lon, elev, timezone
	case SectionRegions:
		return len(m.cfg.ADSB.CollectionRegions) + 1 // regions + "Add New"
	case SectionTelescope:
		return 8 // model, mount_type, imaging_mode, min_alt, max_alt, base_url, slew_rate, tracking_enabled
	case SectionADSB:
		if len(m.cfg.ADSB.Sources) > 0 {
			return 5 // source name, enabled, base_url, rate_limit, search_radius
		}
		return 1
	case SectionDatabase:
		return 5 // driver, host, port, database, username (all read-only)
	default:
		return 0
	}
}

// startEditing begins editing the currently selected field.
func (m *configMenuModel) startEditing() {
	// If editing region, get region field value
	if m.editingRegion && m.currentField < len(m.cfg.ADSB.CollectionRegions) {
		region := m.cfg.ADSB.CollectionRegions[m.currentField]
		switch m.regionEditField {
		case 0:
			m.editBuffer = region.Name
		case 1:
			m.editBuffer = fmt.Sprintf("%.4f", region.Latitude)
		case 2:
			m.editBuffer = fmt.Sprintf("%.4f", region.Longitude)
		case 3:
			m.editBuffer = fmt.Sprintf("%.1f", region.RadiusNM)
		}
		m.editing = true
		m.message = "Editing... (ENTER to save, ESC to cancel)"
		return
	}

	// Get current field value as string
	value := m.getCurrentFieldValue()
	m.editBuffer = value
	m.editing = true
	m.message = "Editing... (ENTER to save, ESC to cancel)"
}

// getCurrentFieldValue returns the current value of the selected field as a string.
func (m *configMenuModel) getCurrentFieldValue() string {
	section := ConfigSection(m.currentSection)

	switch section {
	case SectionGeneral:
		switch m.currentField {
		case 0:
			return m.cfg.Server.Port
		case 1:
			return fmt.Sprintf("%d", m.cfg.ADSB.UpdateIntervalSeconds)
		case 2:
			if len(m.cfg.ADSB.Sources) > 0 {
				return fmt.Sprintf("%.1f", m.cfg.ADSB.Sources[0].RateLimitSeconds)
			}
		}

	case SectionObserver:
		switch m.currentField {
		case 0:
			return m.cfg.Observer.Name
		case 1:
			return fmt.Sprintf("%.4f", m.cfg.Observer.Latitude)
		case 2:
			return fmt.Sprintf("%.4f", m.cfg.Observer.Longitude)
		case 3:
			return fmt.Sprintf("%.1f", m.cfg.Observer.Elevation)
		case 4:
			return m.cfg.Observer.TimeZone
		}

	case SectionTelescope:
		switch m.currentField {
		case 0:
			return m.cfg.Telescope.Model
		case 1:
			return m.cfg.Telescope.MountType
		case 2:
			return m.cfg.Telescope.ImagingMode
		case 3:
			return fmt.Sprintf("%.0f", m.cfg.Telescope.MinAltitude)
		case 4:
			return fmt.Sprintf("%.0f", m.cfg.Telescope.MaxAltitude)
		case 5:
			return m.cfg.Telescope.BaseURL
		case 6:
			return fmt.Sprintf("%.1f", m.cfg.Telescope.SlewRate)
		case 7:
			return fmt.Sprintf("%t", m.cfg.Telescope.TrackingEnabled)
		}

	case SectionADSB:
		if len(m.cfg.ADSB.Sources) > 0 {
			source := m.cfg.ADSB.Sources[0]
			switch m.currentField {
			case 0:
				return source.Name
			case 1:
				return fmt.Sprintf("%t", source.Enabled)
			case 2:
				return source.BaseURL
			case 3:
				return fmt.Sprintf("%.1f", source.RateLimitSeconds)
			case 4:
				return fmt.Sprintf("%.1f", m.cfg.ADSB.SearchRadiusNM)
			}
		}
	}

	return ""
}

// saveFieldValue saves the edited value back to the config.
func (m *configMenuModel) saveFieldValue() error {
	// If editing region, save region field
	if m.editingRegion && m.currentField < len(m.cfg.ADSB.CollectionRegions) {
		value := m.editBuffer
		switch m.regionEditField {
		case 0: // Name
			m.cfg.ADSB.CollectionRegions[m.currentField].Name = value
		case 1: // Latitude
			var lat float64
			if _, err := fmt.Sscanf(value, "%f", &lat); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if lat < -90 || lat > 90 {
				return fmt.Errorf("latitude must be between -90 and +90")
			}
			m.cfg.ADSB.CollectionRegions[m.currentField].Latitude = lat
		case 2: // Longitude
			var lon float64
			if _, err := fmt.Sscanf(value, "%f", &lon); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if lon < -180 || lon > 180 {
				return fmt.Errorf("longitude must be between -180 and +180")
			}
			m.cfg.ADSB.CollectionRegions[m.currentField].Longitude = lon
		case 3: // Radius
			var radius float64
			if _, err := fmt.Sscanf(value, "%f", &radius); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if radius < 1 || radius > 500 {
				return fmt.Errorf("radius must be between 1 and 500 NM")
			}
			m.cfg.ADSB.CollectionRegions[m.currentField].RadiusNM = radius
		}
		return nil
	}

	section := ConfigSection(m.currentSection)
	value := m.editBuffer

	switch section {
	case SectionGeneral:
		switch m.currentField {
		case 0:
			m.cfg.Server.Port = value
		case 1:
			var interval int
			if _, err := fmt.Sscanf(value, "%d", &interval); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if interval < 1 {
				return fmt.Errorf("update interval must be >= 1 second")
			}
			m.cfg.ADSB.UpdateIntervalSeconds = interval
		case 2:
			var rateLimit float64
			if _, err := fmt.Sscanf(value, "%f", &rateLimit); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if rateLimit < 0.1 {
				return fmt.Errorf("rate limit must be >= 0.1 seconds")
			}
			if len(m.cfg.ADSB.Sources) > 0 {
				m.cfg.ADSB.Sources[0].RateLimitSeconds = rateLimit
			}
		}

	case SectionObserver:
		switch m.currentField {
		case 0:
			m.cfg.Observer.Name = value
		case 1:
			var lat float64
			if _, err := fmt.Sscanf(value, "%f", &lat); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if lat < -90 || lat > 90 {
				return fmt.Errorf("latitude must be between -90 and +90")
			}
			m.cfg.Observer.Latitude = lat
		case 2:
			var lon float64
			if _, err := fmt.Sscanf(value, "%f", &lon); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if lon < -180 || lon > 180 {
				return fmt.Errorf("longitude must be between -180 and +180")
			}
			m.cfg.Observer.Longitude = lon
		case 3:
			var elev float64
			if _, err := fmt.Sscanf(value, "%f", &elev); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if elev < 0 || elev > 10000 {
				return fmt.Errorf("elevation must be between 0 and 10000m")
			}
			m.cfg.Observer.Elevation = elev
		case 4:
			m.cfg.Observer.TimeZone = value
		}

	case SectionTelescope:
		switch m.currentField {
		case 0:
			m.cfg.Telescope.Model = value
		case 1:
			if value != "altaz" && value != "equatorial" {
				return fmt.Errorf("mount type must be 'altaz' or 'equatorial'")
			}
			m.cfg.Telescope.MountType = value
		case 2:
			if value != "terrestrial" && value != "astronomical" {
				return fmt.Errorf("imaging mode must be 'terrestrial' or 'astronomical'")
			}
			m.cfg.Telescope.ImagingMode = value
		case 3, 4:
			var alt float64
			if _, err := fmt.Sscanf(value, "%f", &alt); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if alt < 0 || alt > 90 {
				return fmt.Errorf("altitude must be between 0 and 90 degrees")
			}
			if m.currentField == 3 {
				m.cfg.Telescope.MinAltitude = alt
			} else {
				m.cfg.Telescope.MaxAltitude = alt
			}
		case 5:
			m.cfg.Telescope.BaseURL = value
		case 6:
			var rate float64
			if _, err := fmt.Sscanf(value, "%f", &rate); err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			m.cfg.Telescope.SlewRate = rate
		case 7:
			var enabled bool
			if _, err := fmt.Sscanf(value, "%t", &enabled); err != nil {
				return fmt.Errorf("invalid boolean (use 'true' or 'false')")
			}
			m.cfg.Telescope.TrackingEnabled = enabled
		}

	case SectionADSB:
		if len(m.cfg.ADSB.Sources) > 0 {
			switch m.currentField {
			case 0:
				m.cfg.ADSB.Sources[0].Name = value
			case 1:
				var enabled bool
				if _, err := fmt.Sscanf(value, "%t", &enabled); err != nil {
					return fmt.Errorf("invalid boolean (use 'true' or 'false')")
				}
				m.cfg.ADSB.Sources[0].Enabled = enabled
			case 2:
				m.cfg.ADSB.Sources[0].BaseURL = value
			case 3:
				var rateLimit float64
				if _, err := fmt.Sscanf(value, "%f", &rateLimit); err != nil {
					return fmt.Errorf("invalid number: %v", err)
				}
				if rateLimit < 0.1 {
					return fmt.Errorf("rate limit must be >= 0.1 seconds")
				}
				m.cfg.ADSB.Sources[0].RateLimitSeconds = rateLimit
			case 4:
				var radius float64
				if _, err := fmt.Sscanf(value, "%f", &radius); err != nil {
					return fmt.Errorf("invalid number: %v", err)
				}
				if radius < 1 || radius > 500 {
					return fmt.Errorf("search radius must be between 1 and 500 NM")
				}
				m.cfg.ADSB.SearchRadiusNM = radius
			}
		}
	}

	return nil
}

// saveConfig saves the configuration to file.
func (m *configMenuModel) saveConfig() tea.Cmd {
	return func() tea.Msg {
		if err := m.cfg.Save(m.configPath); err != nil {
			return configSaveMsg{err: err}
		}
		return configSaveMsg{success: true}
	}
}

// reloadConfig reloads the configuration from file.
func (m *configMenuModel) reloadConfig() tea.Cmd {
	return func() tea.Msg {
		cfg, err := config.Load(m.configPath)
		if err != nil {
			return configReloadMsg{err: err}
		}
		return configReloadMsg{cfg: cfg}
	}
}

// restoreDefaults resets configuration to defaults.
func (m *configMenuModel) restoreDefaults() {
	m.cfg = config.DefaultConfig()
	m.dirty = true
	m.message = "Defaults restored (not saved)"
}

// toggleRegion toggles the enabled state of the selected region.
func (m *configMenuModel) toggleRegion() {
	if m.currentField < len(m.cfg.ADSB.CollectionRegions) {
		m.cfg.ADSB.CollectionRegions[m.currentField].Enabled = !m.cfg.ADSB.CollectionRegions[m.currentField].Enabled
		m.dirty = true
		state := "disabled"
		if m.cfg.ADSB.CollectionRegions[m.currentField].Enabled {
			state = "enabled"
		}
		m.message = fmt.Sprintf("Region %s %s (not saved)", m.cfg.ADSB.CollectionRegions[m.currentField].Name, state)
		m.messageIsError = false
	}
}

// addNewRegion adds a new collection region with default values.
func (m *configMenuModel) addNewRegion() {
	newRegion := config.CollectionRegion{
		Name:      "New Region",
		Latitude:  m.cfg.Observer.Latitude, // Default to observer location
		Longitude: m.cfg.Observer.Longitude,
		RadiusNM:  100.0,
		Enabled:   false,
	}
	m.cfg.ADSB.CollectionRegions = append(m.cfg.ADSB.CollectionRegions, newRegion)
	m.dirty = true
	m.message = "New region added (not saved). Edit details and save."
	m.messageIsError = false
	// Move selection to the new region
	m.currentField = len(m.cfg.ADSB.CollectionRegions) - 1
}

// deleteRegion removes the selected region.
func (m *configMenuModel) deleteRegion() {
	if m.currentField < len(m.cfg.ADSB.CollectionRegions) {
		regionName := m.cfg.ADSB.CollectionRegions[m.currentField].Name
		// Remove the region
		m.cfg.ADSB.CollectionRegions = append(
			m.cfg.ADSB.CollectionRegions[:m.currentField],
			m.cfg.ADSB.CollectionRegions[m.currentField+1:]...,
		)
		m.dirty = true
		m.message = fmt.Sprintf("Region %s deleted (not saved)", regionName)
		m.messageIsError = false
		// Adjust selection if needed
		if m.currentField >= len(m.cfg.ADSB.CollectionRegions) {
			m.currentField = len(m.cfg.ADSB.CollectionRegions) - 1
			if m.currentField < 0 {
				m.currentField = 0
			}
		}
	}
}

// createObserverFromRegion creates observer position based on selected region.
func (m *configMenuModel) createObserverFromRegion() {
	if m.currentField < len(m.cfg.ADSB.CollectionRegions) {
		region := m.cfg.ADSB.CollectionRegions[m.currentField]

		// Update observer to match region center
		m.cfg.Observer.Name = fmt.Sprintf("%s Observer", region.Name)
		m.cfg.Observer.Latitude = region.Latitude
		m.cfg.Observer.Longitude = region.Longitude
		// Keep existing elevation and timezone as they may be environment-specific

		m.dirty = true
		m.message = fmt.Sprintf("Observer position set to %s center (lat: %.4f°, lon: %.4f°) - not saved",
			region.Name, region.Latitude, region.Longitude)
		m.messageIsError = false
	}
}

// Custom messages
type configSaveMsg struct {
	success bool
	err     error
}

type configReloadMsg struct {
	cfg *config.Config
	err error
}

func (m configMenuModel) View() string {
	var s strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		Padding(0, 1)

	s.WriteString(headerStyle.Render("Configuration Menu"))
	s.WriteString("\n\n")

	// Controls - different for main menu vs submenu
	controlsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	if m.inMainMenu {
		s.WriteString(controlsStyle.Render("[↑/↓] Navigate  [ENTER] Select  [S] Save  [R] Reload  [D] Defaults  [ESC] Exit"))
	} else {
		s.WriteString(controlsStyle.Render("[↑/↓] Navigate  [ENTER] Edit  [ESC] Back to Menu  [S] Save  [R] Reload"))
	}
	s.WriteString("\n")

	// Dirty indicator
	if m.dirty {
		dirtyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
		s.WriteString(dirtyStyle.Render("Modified: * (unsaved changes)"))
	}
	s.WriteString("\n\n")

	// Status message
	if m.message != "" {
		msgStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
		if m.messageIsError {
			msgStyle = msgStyle.Foreground(lipgloss.Color("196"))
		}
		s.WriteString(msgStyle.Render(m.message))
		s.WriteString("\n\n")
	}

	// Render main menu or submenu
	if m.inMainMenu {
		s.WriteString(m.renderMainMenu())
	} else {
		s.WriteString(m.renderSubmenu(ConfigSection(m.currentSection)))
	}

	return s.String()
}

// renderMainMenu renders the main menu showing all sections.
func (m *configMenuModel) renderMainMenu() string {
	var s strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	s.WriteString(headerStyle.Render("━━━ CONFIGURATION SECTIONS ━━━"))
	s.WriteString("\n\n")

	// Section descriptions
	sectionDescriptions := map[ConfigSection]string{
		SectionGeneral:   "Server port, update intervals, and rate limits",
		SectionObserver:  "Observer location (latitude, longitude, elevation)",
		SectionRegions:   "Multi-region aircraft collection areas",
		SectionTelescope: "Telescope model, mount type, and tracking limits",
		SectionADSB:      "ADS-B data sources and search parameters",
		SectionDatabase:  "Database connection (read-only)",
	}

	// Render each section as menu item
	for i := 0; i < int(NumSections); i++ {
		section := ConfigSection(i)
		selected := i == m.currentSection

		// Selection indicator
		prefix := "  "
		if selected {
			prefix = "▸ "
		}

		// Section name style
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		if selected {
			nameStyle = nameStyle.Bold(true).Foreground(lipgloss.Color("51"))
		}

		// Description style
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)

		// Render menu item
		s.WriteString(prefix)
		s.WriteString(nameStyle.Render(section.String()))
		s.WriteString("\n")
		s.WriteString("    ")
		s.WriteString(descStyle.Render(sectionDescriptions[section]))
		s.WriteString("\n\n")
	}

	// Hint
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	s.WriteString("\n")
	s.WriteString(hintStyle.Render("Press ENTER to configure the selected section"))
	s.WriteString("\n")

	return s.String()
}

// renderSubmenu renders a specific configuration section with fields and tooltips.
func (m *configMenuModel) renderSubmenu(section ConfigSection) string {
	var s strings.Builder

	// Section header with breadcrumb
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	s.WriteString(headerStyle.Render(fmt.Sprintf("━━━ %s ━━━", section.String())))
	s.WriteString("\n\n")

	// Render fields based on section
	switch section {
	case SectionGeneral:
		m.renderGeneralSubmenu(&s)
	case SectionObserver:
		m.renderObserverSubmenu(&s)
	case SectionRegions:
		m.renderRegionsSubmenu(&s)
	case SectionTelescope:
		m.renderTelescopeSubmenu(&s)
	case SectionADSB:
		m.renderADSBSubmenu(&s)
	case SectionDatabase:
		m.renderDatabaseSubmenu(&s)
	}

	return s.String()
}

// renderSection renders a configuration section.
func (m *configMenuModel) renderSection(section ConfigSection) string {
	var s strings.Builder

	// Section header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("51"))
	s.WriteString(headerStyle.Render(fmt.Sprintf("━━━ %s ━━━", section.String())))
	s.WriteString("\n")

	// Render fields based on section
	switch section {
	case SectionGeneral:
		m.renderGeneralSection(&s)
	case SectionObserver:
		m.renderObserverSection(&s)
	case SectionRegions:
		m.renderRegionsSection(&s)
	case SectionTelescope:
		m.renderTelescopeSection(&s)
	case SectionADSB:
		m.renderADSBSection(&s)
	case SectionDatabase:
		m.renderDatabaseSection(&s)
	}

	return s.String()
}

// renderGeneralSubmenu renders the General configuration submenu.
func (m *configMenuModel) renderGeneralSubmenu(s *strings.Builder) {
	m.renderFieldWithTooltip(s, 0, "Server Port", m.cfg.Server.Port, "HTTP server port for web interface", "Example: 8080", false)
	m.renderFieldWithTooltip(s, 1, "Update Interval", fmt.Sprintf("%d", m.cfg.ADSB.UpdateIntervalSeconds), "How often to refresh aircraft data (seconds)", "Minimum: 1, Recommended: 10-15", false)
	if len(m.cfg.ADSB.Sources) > 0 {
		m.renderFieldWithTooltip(s, 2, "Rate Limit", fmt.Sprintf("%.1f", m.cfg.ADSB.Sources[0].RateLimitSeconds), "Minimum seconds between API calls", "airplanes.live: 9.0, local SDR: 0.1", false)
	}
}

// renderGeneralSection renders the General configuration section.
func (m *configMenuModel) renderGeneralSection(s *strings.Builder) {
	m.renderField(s, 0, "Server Port", m.cfg.Server.Port, false)
	m.renderField(s, 1, "Update Interval", fmt.Sprintf("%d seconds", m.cfg.ADSB.UpdateIntervalSeconds), false)
	if len(m.cfg.ADSB.Sources) > 0 {
		m.renderField(s, 2, "Rate Limit", fmt.Sprintf("%.1f seconds", m.cfg.ADSB.Sources[0].RateLimitSeconds), false)
	}
}

// renderObserverSubmenu renders the Observer configuration submenu.
func (m *configMenuModel) renderObserverSubmenu(s *strings.Builder) {
	m.renderFieldWithTooltip(s, 0, "Name", m.cfg.Observer.Name, "Friendly name for this observer location", "Example: CLT Primary Observatory", false)
	m.renderFieldWithTooltip(s, 1, "Latitude", fmt.Sprintf("%.4f", m.cfg.Observer.Latitude), "Observer latitude in decimal degrees", "Range: -90 to +90 (North positive)", false)
	m.renderFieldWithTooltip(s, 2, "Longitude", fmt.Sprintf("%.4f", m.cfg.Observer.Longitude), "Observer longitude in decimal degrees", "Range: -180 to +180 (East positive, West negative)", false)
	m.renderFieldWithTooltip(s, 3, "Elevation", fmt.Sprintf("%.1f", m.cfg.Observer.Elevation), "Elevation above sea level (meters)", "Range: 0 to 10000", false)
	m.renderFieldWithTooltip(s, 4, "Timezone", m.cfg.Observer.TimeZone, "IANA timezone name", "Example: America/New_York, America/Los_Angeles", false)
}

// renderObserverSection renders the Observer configuration section.
func (m *configMenuModel) renderObserverSection(s *strings.Builder) {
	m.renderField(s, 0, "Name", m.cfg.Observer.Name, false)
	m.renderField(s, 1, "Latitude", fmt.Sprintf("%.4f°N", m.cfg.Observer.Latitude), false)
	m.renderField(s, 2, "Longitude", fmt.Sprintf("%.4f°W", m.cfg.Observer.Longitude), false)
	m.renderField(s, 3, "Elevation", fmt.Sprintf("%.1fm", m.cfg.Observer.Elevation), false)
	m.renderField(s, 4, "Timezone", m.cfg.Observer.TimeZone, false)
}

// renderTelescopeSubmenu renders the Telescope configuration submenu.
func (m *configMenuModel) renderTelescopeSubmenu(s *strings.Builder) {
	m.renderFieldWithTooltip(s, 0, "Model", m.cfg.Telescope.Model, "Telescope model identifier", "Example: seestar-s50, seestar-s30, generic", false)
	m.renderFieldWithTooltip(s, 1, "Mount Type", m.cfg.Telescope.MountType, "Mount type for coordinate system", "Values: altaz, equatorial", false)
	m.renderFieldWithTooltip(s, 2, "Imaging Mode", m.cfg.Telescope.ImagingMode, "Operational mode for altitude limits", "Values: terrestrial (0° min), astronomical (15° min)", false)
	m.renderFieldWithTooltip(s, 3, "Min Altitude", fmt.Sprintf("%.0f", m.cfg.Telescope.MinAltitude), "Minimum tracking altitude (degrees)", "0 = auto-detect, typical: 0-20°", false)
	m.renderFieldWithTooltip(s, 4, "Max Altitude", fmt.Sprintf("%.0f", m.cfg.Telescope.MaxAltitude), "Maximum tracking altitude (degrees)", "0 = auto-detect, typical: 80-85°", false)
	m.renderFieldWithTooltip(s, 5, "Base URL", m.cfg.Telescope.BaseURL, "ASCOM Alpaca server URL", "Example: http://localhost:11111", false)
	m.renderFieldWithTooltip(s, 6, "Slew Rate", fmt.Sprintf("%.1f", m.cfg.Telescope.SlewRate), "Slew speed (degrees/second)", "Typical: 0.5-3.0", false)
	m.renderFieldWithTooltip(s, 7, "Tracking Enabled", fmt.Sprintf("%t", m.cfg.Telescope.TrackingEnabled), "Enable automatic tracking", "Values: true, false", false)
}

// renderADSBSubmenu renders the ADS-B configuration submenu.
func (m *configMenuModel) renderADSBSubmenu(s *strings.Builder) {
	if len(m.cfg.ADSB.Sources) > 0 {
		source := m.cfg.ADSB.Sources[0]
		m.renderFieldWithTooltip(s, 0, "Source Name", source.Name, "ADS-B data source identifier", "Example: airplanes.live, local-sdr", false)
		m.renderFieldWithTooltip(s, 1, "Enabled", fmt.Sprintf("%t", source.Enabled), "Enable this data source", "Values: true, false", false)
		m.renderFieldWithTooltip(s, 2, "Base URL", source.BaseURL, "API base URL for online sources", "airplanes.live: https://api.airplanes.live/v2", false)
		m.renderFieldWithTooltip(s, 3, "Rate Limit", fmt.Sprintf("%.1f", source.RateLimitSeconds), "Minimum seconds between API calls", "airplanes.live: 9.0, local: 0.1-1.0", false)
		m.renderFieldWithTooltip(s, 4, "Search Radius", fmt.Sprintf("%.1f", m.cfg.ADSB.SearchRadiusNM), "Default search radius (nautical miles)", "Range: 1-500", false)
	}
}

// renderRegionsSubmenu renders the Collection Regions submenu.
func (m *configMenuModel) renderRegionsSubmenu(s *strings.Builder) {
	// If editing a specific region, show region edit interface
	if m.editingRegion && m.currentField < len(m.cfg.ADSB.CollectionRegions) {
		m.renderRegionEditor(s)
		return
	}

	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	s.WriteString(hintStyle.Render("Collection regions allow fetching aircraft from multiple areas."))
	s.WriteString("\n")
	s.WriteString(hintStyle.Render("[SPACE] Toggle  [ENTER] Edit  [O] Set Observer  [X] Delete"))
	s.WriteString("\n\n")

	for i, region := range m.cfg.ADSB.CollectionRegions {
		checkbox := "[ ]"
		if region.Enabled {
			checkbox = "[✓]"
		}
		label := fmt.Sprintf("%s %s", checkbox, region.Name)
		details := fmt.Sprintf("    %.4f°N, %.4f°W, Radius: %.0f NM", region.Latitude, region.Longitude, region.RadiusNM)

		selected := i == m.currentField
		prefix := "  "
		if selected {
			prefix = "▸ "
		}

		fieldStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		if selected {
			fieldStyle = fieldStyle.Background(lipgloss.Color("237"))
		}

		s.WriteString(fieldStyle.Render(prefix + label))
		s.WriteString("\n")
		s.WriteString(fieldStyle.Render("  " + details))
		s.WriteString("\n\n")
	}

	// Add new region option
	selected := m.currentField == len(m.cfg.ADSB.CollectionRegions)
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	fieldStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	if selected {
		fieldStyle = fieldStyle.Background(lipgloss.Color("237"))
	}
	s.WriteString(fieldStyle.Render(prefix + "[+] Add New Region"))
	s.WriteString("\n")
}

// renderDatabaseSubmenu renders the Database configuration submenu (read-only).
func (m *configMenuModel) renderDatabaseSubmenu(s *strings.Builder) {
	m.renderFieldWithTooltip(s, 0, "Driver", m.cfg.Database.Driver, "Database driver", "Supported: postgres", true)
	m.renderFieldWithTooltip(s, 1, "Host", m.cfg.Database.Host, "Database server hostname", "Configured via ADS_BSCOPE_DB_HOST", true)
	m.renderFieldWithTooltip(s, 2, "Port", fmt.Sprintf("%d", m.cfg.Database.Port), "Database server port", "Default: 5432", true)
	m.renderFieldWithTooltip(s, 3, "Database", m.cfg.Database.Database, "Database name", "Default: adsbscope", true)
	m.renderFieldWithTooltip(s, 4, "Username", m.cfg.Database.Username, "Database username", "Default: adsbscope", true)
	s.WriteString("\n")
	readOnlyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	s.WriteString(readOnlyStyle.Render("  ⚠️  Database settings are read-only. Edit via environment variables:"))
	s.WriteString("\n")
	s.WriteString(readOnlyStyle.Render("  ADS_BSCOPE_DB_HOST, ADS_BSCOPE_DB_PASSWORD"))
	s.WriteString("\n")
}

// renderRegionsSection renders the Collection Regions section.
func (m *configMenuModel) renderRegionsSection(s *strings.Builder) {
	for i, region := range m.cfg.ADSB.CollectionRegions {
		checkbox := "[ ]"
		if region.Enabled {
			checkbox = "[✓]"
		}
		label := fmt.Sprintf("%s %s (%.0f NM)", checkbox, region.Name, region.RadiusNM)
		m.renderField(s, i, "", label, false)
	}
	m.renderField(s, len(m.cfg.ADSB.CollectionRegions), "", "[+] Add New Region", false)
}

// renderTelescopeSection renders the Telescope configuration section.
func (m *configMenuModel) renderTelescopeSection(s *strings.Builder) {
	m.renderField(s, 0, "Model", m.cfg.Telescope.Model, false)
	m.renderField(s, 1, "Mount Type", m.cfg.Telescope.MountType, false)
	m.renderField(s, 2, "Imaging Mode", m.cfg.Telescope.ImagingMode, false)
	m.renderField(s, 3, "Min Altitude", fmt.Sprintf("%.0f°", m.cfg.Telescope.MinAltitude), false)
	m.renderField(s, 4, "Max Altitude", fmt.Sprintf("%.0f°", m.cfg.Telescope.MaxAltitude), false)
	m.renderField(s, 5, "Base URL", m.cfg.Telescope.BaseURL, false)
	m.renderField(s, 6, "Slew Rate", fmt.Sprintf("%.1f", m.cfg.Telescope.SlewRate), false)
	m.renderField(s, 7, "Tracking Enabled", fmt.Sprintf("%t", m.cfg.Telescope.TrackingEnabled), false)
}

// renderADSBSection renders the ADS-B configuration section.
func (m *configMenuModel) renderADSBSection(s *strings.Builder) {
	if len(m.cfg.ADSB.Sources) > 0 {
		source := m.cfg.ADSB.Sources[0]
		m.renderField(s, 0, "Source Name", source.Name, false)
		m.renderField(s, 1, "Enabled", fmt.Sprintf("%t", source.Enabled), false)
		m.renderField(s, 2, "Base URL", source.BaseURL, false)
		m.renderField(s, 3, "Rate Limit", fmt.Sprintf("%.1f seconds", source.RateLimitSeconds), false)
		m.renderField(s, 4, "Search Radius", fmt.Sprintf("%.1f NM", m.cfg.ADSB.SearchRadiusNM), false)
	}
}

// renderDatabaseSection renders the Database configuration section (read-only).
func (m *configMenuModel) renderDatabaseSection(s *strings.Builder) {
	m.renderField(s, 0, "Driver", m.cfg.Database.Driver, true)
	m.renderField(s, 1, "Host", m.cfg.Database.Host, true)
	m.renderField(s, 2, "Port", fmt.Sprintf("%d", m.cfg.Database.Port), true)
	m.renderField(s, 3, "Database", m.cfg.Database.Database, true)
	m.renderField(s, 4, "Username", m.cfg.Database.Username, true)
	s.WriteString("\n")
	readOnlyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	s.WriteString(readOnlyStyle.Render("  (Edit via environment variables or config file)"))
	s.WriteString("\n")
}

// renderField renders a single configuration field.
func (m *configMenuModel) renderField(s *strings.Builder, fieldIndex int, label, value string, readOnly bool) {
	selected := ConfigSection(m.currentSection) == ConfigSection(m.currentSection) && m.currentField == fieldIndex

	// Selection indicator
	prefix := "  "
	if selected {
		prefix = "▸ "
	}

	// Formatting
	fieldStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	if readOnly {
		fieldStyle = fieldStyle.Foreground(lipgloss.Color("240"))
	}
	if selected && !readOnly {
		fieldStyle = fieldStyle.Background(lipgloss.Color("237"))
	}

	// If editing this field, show edit buffer
	displayValue := value
	if selected && m.editing {
		displayValue = m.editBuffer + "_"
	}

	// Format line
	line := prefix
	if label != "" {
		line += label + ": " + displayValue
	} else {
		line += displayValue
	}

	s.WriteString(fieldStyle.Render(line))
	s.WriteString("\n")
}

// renderRegionEditor renders the region editing interface.
func (m *configMenuModel) renderRegionEditor(s *strings.Builder) {
	region := m.cfg.ADSB.CollectionRegions[m.currentField]

	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	s.WriteString(hintStyle.Render(fmt.Sprintf("Editing Region: %s", region.Name)))
	s.WriteString("\n\n")

	// Field labels and values
	fields := []struct {
		label       string
		value       string
		description string
		example     string
	}{
		{"Name", region.Name, "Region identifier", "Example: Charlotte Metro"},
		{"Latitude", fmt.Sprintf("%.4f", region.Latitude), "Center latitude in decimal degrees", "Range: -90 to +90"},
		{"Longitude", fmt.Sprintf("%.4f", region.Longitude), "Center longitude in decimal degrees", "Range: -180 to +180"},
		{"Radius", fmt.Sprintf("%.1f NM", region.RadiusNM), "Collection radius in nautical miles", "Range: 1 to 500"},
	}

	for i, field := range fields {
		selected := i == m.regionEditField

		// Selection indicator
		prefix := "  "
		if selected {
			prefix = "▸ "
		}

		// Field style
		fieldStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
		if selected {
			fieldStyle = fieldStyle.Bold(true).Foreground(lipgloss.Color("51"))
		}

		// If editing this field, show edit buffer
		displayValue := field.value
		if selected && m.editing {
			displayValue = m.editBuffer + "_"
		}

		// Field label and value
		line := fmt.Sprintf("%s%s: %s", prefix, field.label, displayValue)
		s.WriteString(fieldStyle.Render(line))
		s.WriteString("\n")

		// Show tooltip for selected field
		if selected {
			tooltipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
			s.WriteString("    ")
			s.WriteString(tooltipStyle.Render("• " + field.description))
			s.WriteString("\n")
			s.WriteString("    ")
			s.WriteString(tooltipStyle.Render("• " + field.example))
			s.WriteString("\n")
		}

		s.WriteString("\n")
	}
}

// renderFieldWithTooltip renders a field with description and example tooltip.
func (m *configMenuModel) renderFieldWithTooltip(s *strings.Builder, fieldIndex int, label, value, description, example string, readOnly bool) {
	selected := m.currentField == fieldIndex

	// Selection indicator
	prefix := "  "
	if selected {
		prefix = "▸ "
	}

	// Field style
	fieldStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	if readOnly {
		fieldStyle = fieldStyle.Foreground(lipgloss.Color("240"))
	}
	if selected && !readOnly {
		fieldStyle = fieldStyle.Bold(true).Foreground(lipgloss.Color("51"))
	}

	// If editing this field, show edit buffer
	displayValue := value
	if selected && m.editing {
		displayValue = m.editBuffer + "_"
	}

	// Field label and value
	line := fmt.Sprintf("%s%s: %s", prefix, label, displayValue)
	s.WriteString(fieldStyle.Render(line))
	s.WriteString("\n")

	// Show tooltip for selected field
	if selected {
		tooltipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
		s.WriteString("    ")
		s.WriteString(tooltipStyle.Render("• " + description))
		s.WriteString("\n")
		s.WriteString("    ")
		s.WriteString(tooltipStyle.Render("• " + example))
		s.WriteString("\n")
	}

	s.WriteString("\n")
}
