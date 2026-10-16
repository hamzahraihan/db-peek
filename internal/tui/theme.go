package tui

import (
	"strings"
)

// Palette defines the semantic colors used across peekerdb.
// Colors are hex ("#RRGGBB") or ANSI color index ("8", "15", "236") strings.
type Palette struct {
	Primary          string // Main accent: PKs, active buffers, explorer titles
	Secondary        string // Secondary accent: column types, help keys, FKs
	Success          string // Positive state: unique constraints, SQL strings
	Error            string // Negative state: error text, delete warnings
	Text             string // Bright primary text: headers, fields, editor text
	TextMuted        string // Dim/secondary text: counts, metadata, comments
	TextDimField     string // Unfocused field text (slightly brighter than muted)

	ActiveTabBg      string // Active tab background pill
	ActiveTabFg      string // Active tab text
	InactiveTabBg    string // Inactive tab background
	InactiveTabFg    string // Inactive tab text
	HoverTabBg       string // Tab hover background
	HoverTabFg       string // Tab hover text

	SelTitleBg       string // Picker selection background
	SelTitleFg       string // Picker selection title text
	SelDescBg        string // Picker selection description background
	SelDescFg        string // Picker selection description text

	DataHeaderFg     string // Grid header foreground
	DataHeaderBorder string // Grid header separator
	DataSelBg        string // Grid selected row background
	DataSelFg        string // Grid selected row text
	DataHoverBg      string // Grid hovered row background
	DataHoverFg      string // Grid hovered row text
	ZebraBg          string // Grid alternating row background
	ZebraDimBg       string // Grid unfocused alternating row background

	ExplorerTitle    string // Sidebar tree title
	ExplorerSelBg    string // Sidebar tree selected row background
	ExplorerSelFg    string // Sidebar tree selected row text
	ExplorerType     string // Sidebar column type text
	ExplorerCount    string // Sidebar count text

	BorderActive     string // Focused pane border
	BorderInactive   string // Unfocused pane border

	ERBoxBorder      string // ER diagram table border
	ERBoxSelBorder   string // ER diagram selected table border
	ERBoxHover       string // ER diagram hovered table border
	ERLink           string // ER diagram relationship link glyphs

	SQLKeyword       string // SQL syntax keywords
	SQLString        string // SQL syntax strings
	SQLNumber        string // SQL syntax numbers
	SQLComment       string // SQL syntax comments
	SQLPlain         string // SQL syntax plain text
}

// Theme encapsulates a named color scheme with metadata.
type Theme struct {
	ID          string
	Name        string
	Description string
	Palette     Palette
}

// PaletteDefault preserves peekerdb's original classic dark styling verbatim.
var PaletteDefault = Palette{
	Primary:          "#EAB308",
	Secondary:        "#67E8F9",
	Success:          "#4ADE80",
	Error:            "9",
	Text:             "15",
	TextMuted:        "8",
	TextDimField:     "7",

	ActiveTabBg:      "#005FD7",
	ActiveTabFg:      "#FFFFFF",
	InactiveTabBg:    "236",
	InactiveTabFg:    "8",
	HoverTabBg:       "236",
	HoverTabFg:       "#00D7FF",

	SelTitleBg:       "62",
	SelTitleFg:       "230",
	SelDescBg:        "62",
	SelDescFg:        "254",

	DataHeaderFg:     "12",
	DataHeaderBorder: "8",
	DataSelBg:        "62",
	DataSelFg:        "230",
	DataHoverBg:      "236",
	DataHoverFg:      "252",
	ZebraBg:          "235",
	ZebraDimBg:       "234",

	ExplorerTitle:    "#EAB308",
	ExplorerSelBg:    "#CA8A04",
	ExplorerSelFg:    "#000000",
	ExplorerType:     "#67E8F9",
	ExplorerCount:    "8",

	BorderActive:     "#EAB308",
	BorderInactive:   "#3A3A3A",

	ERBoxBorder:      "#005FD7",
	ERBoxSelBorder:   "#CA8A04",
	ERBoxHover:       "#00D7FF",
	ERLink:           "8",

	SQLKeyword:       "#EAB308",
	SQLString:        "#4ADE80",
	SQLNumber:        "#67E8F9",
	SQLComment:       "8",
	SQLPlain:         "15",
}

// PaletteKanagawa is inspired by the iconic Japanese art palette by rebelot.
var PaletteKanagawa = Palette{
	Primary:          "#E6C384", // Carp Yellow
	Secondary:        "#7AA89F", // Wave Aqua
	Success:          "#98BB6C", // Spring Green
	Error:            "#E46876", // Autumn Red
	Text:             "#DCD7BA", // Fuji White
	TextMuted:        "#727169", // Fuji Gray
	TextDimField:     "#C8C093", // Old White

	ActiveTabBg:      "#7E9CD8", // Crystal Blue
	ActiveTabFg:      "#16161D", // Sumisora Dark
	InactiveTabBg:    "#2A2A37", // Sumisora 2
	InactiveTabFg:    "#727169",
	HoverTabBg:       "#2A2A37",
	HoverTabFg:       "#7FB4CA", // Spring Blue

	SelTitleBg:       "#2D4F67", // Wave Blue 2
	SelTitleFg:       "#DCD7BA",
	SelDescBg:        "#2D4F67",
	SelDescFg:        "#C8C093",

	DataHeaderFg:     "#7E9CD8",
	DataHeaderBorder: "#727169",
	DataSelBg:        "#2D4F67",
	DataSelFg:        "#DCD7BA",
	DataHoverBg:      "#223249", // Wave Blue 1
	DataHoverFg:      "#DCD7BA",
	ZebraBg:          "#22222E",
	ZebraDimBg:       "#1A1A24",

	ExplorerTitle:    "#E6C384",
	ExplorerSelBg:    "#E6C384",
	ExplorerSelFg:    "#16161D",
	ExplorerType:     "#7AA89F",
	ExplorerCount:    "#727169",

	BorderActive:     "#E6C384",
	BorderInactive:   "#363646",

	ERBoxBorder:      "#7E9CD8",
	ERBoxSelBorder:   "#E6C384",
	ERBoxHover:       "#7AA89F",
	ERLink:           "#54546D",

	SQLKeyword:       "#957FB8", // Oni Violet
	SQLString:        "#98BB6C", // Spring Green
	SQLNumber:        "#D27E99", // Sakura Pink
	SQLComment:       "#727169",
	SQLPlain:         "#DCD7BA",
}

// PaletteCatppuccin implements the soothing, warm Catppuccin Mocha palette.
var PaletteCatppuccin = Palette{
	Primary:          "#F9E2AF", // Yellow
	Secondary:        "#89DCEB", // Sky
	Success:          "#A6E3A1", // Green
	Error:            "#F38BA8", // Red
	Text:             "#CDD6F4", // Text
	TextMuted:        "#6C7086", // Overlay0
	TextDimField:     "#A6ADC8", // Subtext0

	ActiveTabBg:      "#89B4FA", // Blue
	ActiveTabFg:      "#11111B", // Crust
	InactiveTabBg:    "#313244", // Surface0
	InactiveTabFg:    "#6C7086",
	HoverTabBg:       "#313244",
	HoverTabFg:       "#74C7EC", // Sapphire

	SelTitleBg:       "#45475A", // Surface1
	SelTitleFg:       "#CDD6F4",
	SelDescBg:        "#45475A",
	SelDescFg:        "#BAC2DE",

	DataHeaderFg:     "#89B4FA",
	DataHeaderBorder: "#6C7086",
	DataSelBg:        "#45475A",
	DataSelFg:        "#CDD6F4",
	DataHoverBg:      "#313244",
	DataHoverFg:      "#CDD6F4",
	ZebraBg:          "#24273A",
	ZebraDimBg:       "#1E1E2E",

	ExplorerTitle:    "#F9E2AF",
	ExplorerSelBg:    "#F9E2AF",
	ExplorerSelFg:    "#11111B",
	ExplorerType:     "#89DCEB",
	ExplorerCount:    "#6C7086",

	BorderActive:     "#F9E2AF",
	BorderInactive:   "#45475A",

	ERBoxBorder:      "#89B4FA",
	ERBoxSelBorder:   "#F9E2AF",
	ERBoxHover:       "#89DCEB",
	ERLink:           "#585B70",

	SQLKeyword:       "#CBA6F7", // Mauve
	SQLString:        "#A6E3A1", // Green
	SQLNumber:        "#FAB387", // Peach
	SQLComment:       "#6C7086",
	SQLPlain:         "#CDD6F4",
}

// PaletteOneDark implements Atom's iconic One Dark palette.
var PaletteOneDark = Palette{
	Primary:          "#E5C07B", // Yellow
	Secondary:        "#56B6C2", // Cyan
	Success:          "#98C379", // Green
	Error:            "#E06C75", // Red
	Text:             "#ABB2BF", // Foreground
	TextMuted:        "#5C6370", // Comment / Muted
	TextDimField:     "#828997",

	ActiveTabBg:      "#61AFEF", // Blue
	ActiveTabFg:      "#282C34", // Bg
	InactiveTabBg:    "#21252B",
	InactiveTabFg:    "#5C6370",
	HoverTabBg:       "#21252B",
	HoverTabFg:       "#56B6C2",

	SelTitleBg:       "#3E4451",
	SelTitleFg:       "#ABB2BF",
	SelDescBg:        "#3E4451",
	SelDescFg:        "#828997",

	DataHeaderFg:     "#61AFEF",
	DataHeaderBorder: "#5C6370",
	DataSelBg:        "#3E4451",
	DataSelFg:        "#ABB2BF",
	DataHoverBg:      "#2C313C",
	DataHoverFg:      "#ABB2BF",
	ZebraBg:          "#2C313C",
	ZebraDimBg:       "#21252B",

	ExplorerTitle:    "#E5C07B",
	ExplorerSelBg:    "#E5C07B",
	ExplorerSelFg:    "#282C34",
	ExplorerType:     "#56B6C2",
	ExplorerCount:    "#5C6370",

	BorderActive:     "#61AFEF",
	BorderInactive:   "#3E4451",

	ERBoxBorder:      "#61AFEF",
	ERBoxSelBorder:   "#E5C07B",
	ERBoxHover:       "#56B6C2",
	ERLink:           "#4B5263",

	SQLKeyword:       "#C678DD", // Purple
	SQLString:        "#98C379", // Green
	SQLNumber:        "#D19A66", // Orange
	SQLComment:       "#5C6370",
	SQLPlain:         "#ABB2BF",
}

// PaletteNightOwl implements Sarah Drasner's vibrant Night Owl palette.
var PaletteNightOwl = Palette{
	Primary:          "#ECC48D", // Yellow/Gold
	Secondary:        "#7FDBCA", // Cyan
	Success:          "#ADDB67", // Light Green
	Error:            "#EF5350", // Red
	Text:             "#D6DEEB", // White
	TextMuted:        "#637777", // Muted teal-gray
	TextDimField:     "#7E97B0",

	ActiveTabBg:      "#82AAFF", // Blue
	ActiveTabFg:      "#011627", // Deep Navy
	InactiveTabBg:    "#0B253A",
	InactiveTabFg:    "#637777",
	HoverTabBg:       "#0B253A",
	HoverTabFg:       "#7FDBCA",

	SelTitleBg:       "#1D3B53",
	SelTitleFg:       "#D6DEEB",
	SelDescBg:        "#1D3B53",
	SelDescFg:        "#7E97B0",

	DataHeaderFg:     "#82AAFF",
	DataHeaderBorder: "#637777",
	DataSelBg:        "#1D3B53",
	DataSelFg:        "#D6DEEB",
	DataHoverBg:      "#0B253A",
	DataHoverFg:      "#D6DEEB",
	ZebraBg:          "#082035",
	ZebraDimBg:       "#011627",

	ExplorerTitle:    "#ECC48D",
	ExplorerSelBg:    "#ECC48D",
	ExplorerSelFg:    "#011627",
	ExplorerType:     "#7FDBCA",
	ExplorerCount:    "#637777",

	BorderActive:     "#82AAFF",
	BorderInactive:   "#1D3B53",

	ERBoxBorder:      "#82AAFF",
	ERBoxSelBorder:   "#ECC48D",
	ERBoxHover:       "#7FDBCA",
	ERLink:           "#234D70",

	SQLKeyword:       "#C792EA", // Purple
	SQLString:        "#ECC48D", // Yellow
	SQLNumber:        "#F78C6C", // Orange
	SQLComment:       "#637777",
	SQLPlain:         "#D6DEEB",
}

// AvailableThemes is the list of all registered themes available in the picker.
var AvailableThemes = []Theme{
	{
		ID:          "default",
		Name:        "Default",
		Description: "PeekerDB classic",
		Palette:     PaletteDefault,
	},
	{
		ID:          "kanagawa",
		Name:        "Kanagawa",
		Description: "Wave palette",
		Palette:     PaletteKanagawa,
	},
	{
		ID:          "catppuccin",
		Name:        "Catppuccin",
		Description: "Soothing mocha",
		Palette:     PaletteCatppuccin,
	},
	{
		ID:          "onedark",
		Name:        "One Dark",
		Description: "Atom One Dark",
		Palette:     PaletteOneDark,
	},
	{
		ID:          "nightowl",
		Name:        "Night Owl",
		Description: "Deep navy palette",
		Palette:     PaletteNightOwl,
	},
}

// CanonicalThemeID normalizes user input and theme IDs for matching.
// Accommodates typos/aliases like "catpuccin", "one dark", "night owl", etc.
func CanonicalThemeID(raw string) string {
	norm := strings.ToLower(strings.TrimSpace(raw))
	norm = strings.ReplaceAll(norm, "-", "")
	norm = strings.ReplaceAll(norm, "_", "")
	norm = strings.ReplaceAll(norm, " ", "")
	switch norm {
	case "kanagawa":
		return "kanagawa"
	case "catpuccin", "catppuccin", "mocha", "catppuccinmocha":
		return "catppuccin"
	case "onedark", "atomonedark", "atom":
		return "onedark"
	case "nightowl", "night":
		return "nightowl"
	case "default", "classic", "":
		return "default"
	default:
		return "default"
	}
}

// GetTheme returns the Theme matching the given ID or name, defaulting to Default.
func GetTheme(id string) Theme {
	canon := CanonicalThemeID(id)
	for _, t := range AvailableThemes {
		if t.ID == canon {
			return t
		}
	}
	return AvailableThemes[0]
}

// CurrentTheme stores the currently active theme in peekerdb.
var CurrentTheme = AvailableThemes[0]

// ApplyTheme sets the active theme by ID or alias and recomputes all styling.
func ApplyTheme(id string) Theme {
	t := GetTheme(id)
	CurrentTheme = t
	applyPalette(t.Palette)
	return t
}
