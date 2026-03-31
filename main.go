package main

import (
	_ "embed"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"gopkg.in/yaml.v3"
)

//go:embed font.ttf
var defaultFont []byte

type Theme struct {
	Resources map[string]string `yaml:"resources"`
	Colors    map[string]string `yaml:"colors"`
	Board     map[string]string `yaml:"board"`
	UI        map[string]string `yaml:"ui"`
	Widths    map[string]int    `yaml:"widths"`
}

type ThemeConfig struct {
	Active     string            `yaml:"active"`
	Characters map[string]string `yaml:"characters"`
	Themes     map[string]Theme  `yaml:"themes"`
}

var activeTheme = Theme{
	Resources: map[string]string{
		"wood": "W", "brick": "B", "sheep": "s", "wheat": "w", "ore": "O", "desert": "D",
	},
	Colors: map[string]string{
		"wood": "#795548", "brick": "#c62828", "sheep": "#9ccc65", "wheat": "#fbc02d", "ore": "#78909c", "desert": "#555555",
	},
	Board: map[string]string{
		"port": "S", "settlement": "o", "city": "H", "robber": "X",
	},
	UI: map[string]string{
		"playing": ">>", "paused": "||", "largest_army": "[A]", "longest_road": "[R]", "player_cursor": ">>",
	},
}

func loadTheme() {
	data, err := os.ReadFile("themes.yaml")
	if err != nil {
		return // Use default
	}
	var config ThemeConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return
	}
	if theme, ok := config.Themes[config.Active]; ok {
		// Resolve character mappings
		resolveChars := func(m map[string]string) {
			for k, v := range m {
				if actual, ok := config.Characters[v]; ok {
					m[k] = actual
				}
			}
		}
		resolveChars(theme.Resources)
		resolveChars(theme.Board)
		resolveChars(theme.UI)

		activeTheme = theme
		// Update resource styles with new icons and colors
		for k, v := range resourceStyles {
			if icon, ok := activeTheme.Resources[k]; ok {
				v.Icon = icon
			}
			if hex, ok := activeTheme.Colors[k]; ok {
				v.Color = lipgloss.Color(hex)
			}
			resourceStyles[k] = v
		}
	}
}

var (
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	dashboardStyle = lipgloss.NewStyle().
			Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // Yellow
			Bold(true).
			Underline(true).
			MarginBottom(1)

	activePlayerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")). // Green
				Bold(true)

	playerColorStyles = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),  // Red
		lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true), // Green
		lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true), // Cyan (replaces hard-to-read blue)
		lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true), // Yellow
	}

	resourceStyles = map[string]struct {
		Icon  string
		Color lipgloss.Color
	}{
		"wood":   {activeTheme.Resources["wood"], lipgloss.Color(activeTheme.Colors["wood"])},
		"brick":  {activeTheme.Resources["brick"], lipgloss.Color(activeTheme.Colors["brick"])},
		"sheep":  {activeTheme.Resources["sheep"], lipgloss.Color(activeTheme.Colors["sheep"])},
		"wheat":  {activeTheme.Resources["wheat"], lipgloss.Color(activeTheme.Colors["wheat"])},
		"ore":    {activeTheme.Resources["ore"], lipgloss.Color(activeTheme.Colors["ore"])},
		"desert": {activeTheme.Resources["desert"], lipgloss.Color(activeTheme.Colors["desert"])},
	}

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("15")).
			Bold(true)
)

const (
	MaxRoads       = 15
	MaxSettlements = 5
	MaxCities      = 4
	MaxBank        = 19
)

// GameState structs (matching catan-cli/game.yaml)
type GameState struct {
	Meta    Meta                `yaml:"meta"`
	Board   BoardState          `yaml:"board"`
	Players []Player            `yaml:"players"`
	Log     []LogEntry          `yaml:"log"`
}

type Meta struct {
	Status              string       `yaml:"status"`
	LastActionTimestamp int64        `yaml:"last_action_timestamp"`
	TimeoutSeconds      int          `yaml:"timeout_seconds"`
	MaxSkips            int          `yaml:"max_skips"`
	TurnOrder           []string     `yaml:"turn_order"`
	CurrentPlayerID     string       `yaml:"current_player_id"`
	Phase               string       `yaml:"phase"`
	DevCardDeck         []string     `yaml:"dev_card_deck"`
	PendingOffers       []TradeOffer `yaml:"pending_offers"`
	DiscardingPlayers   []string     `yaml:"discarding_players"`
	LargestArmyPlayer   string       `yaml:"largest_army_player"`
	LargestArmyCount    int          `yaml:"largest_army_count"`
	LongestRoadPlayer   string       `yaml:"longest_road_player"`
	LongestRoadCount    int          `yaml:"longest_road_count"`
	LastRoll1           int          `yaml:"last_roll_1"`
	LastRoll2           int          `yaml:"last_roll_2"`
}

type TradeOffer struct {
	ID           string `yaml:"id"`
	FromPlayerID string `yaml:"from_player_id"`
	GiveResource string `yaml:"give_resource"`
	GiveAmount   int    `yaml:"give_amount"`
	GetResource  string `yaml:"get_resource"`
	GetAmount    int    `yaml:"get_amount"`
}

type BoardState struct {
	Hexes    map[string]HexState    `yaml:"hexes"`
	Vertices map[string]VertexState `yaml:"vertices"`
	Edges    map[string]EdgeState   `yaml:"edges"`
	Ports    map[string]string      `yaml:"ports"` // id -> type
}

type HexState struct {
	Resource string `yaml:"resource"`
	Token    int    `yaml:"token"`
	Robber   bool   `yaml:"robber"`
	Vertices string `yaml:"vertices"`
}

type VertexState struct {
	OwnerID string `yaml:"owner_id"`
	Type    string `yaml:"type"`
}

type EdgeState struct {
	OwnerID string `yaml:"owner_id"`
}

type Player struct {
	ID            string         `yaml:"id"`
	Type          string         `yaml:"type"` // "git", "guest", "bot"
	Resources     map[string]int `yaml:"resources"`
	VP            int            `yaml:"vp"`
	SkipCount     int            `yaml:"skip_count"`
	DevCards      map[string]int `yaml:"dev_cards"`
	NewDevCards   map[string]int `yaml:"new_dev_cards"` // Bought this turn
	KnightsPlayed int            `yaml:"knights_played"`
}

type LogEntry struct {
	Timestamp int64  `yaml:"timestamp"`
	PlayerID  string `yaml:"player_id"`
	Action    string `yaml:"action"`
	Data      string `yaml:"data"`
}

type Cell struct {
	Rune rune
	FG   color.RGBA
	BG   color.RGBA
	Bold bool
}

type GridBuffer struct {
	Width  int
	Height int
	Cells  [][]Cell
}

func NewGridBuffer(w, h int) *GridBuffer {
	cells := make([][]Cell, h)
	for i := range cells {
		cells[i] = make([]Cell, w)
		for j := range cells[i] {
			cells[i][j] = Cell{
				Rune: ' ',
				FG:   color.RGBA{255, 255, 255, 255},
				BG:   color.RGBA{30, 30, 30, 255},
			}
		}
	}
	return &GridBuffer{Width: w, Height: h, Cells: cells}
}

var BuildCosts = map[string]map[string]int{
	"road":       {"wood": 1, "brick": 1},
	"settlement": {"wood": 1, "brick": 1, "sheep": 1, "wheat": 1},
	"city":       {"wheat": 2, "ore": 3},
	"dev_card":   {"sheep": 1, "wheat": 1, "ore": 1},
}

func (s *GameState) Init(topo *Topology) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	s.Meta.Status = "invitation"
	s.Meta.LastActionTimestamp = time.Now().Unix()
	s.Meta.TimeoutSeconds = 86400
	s.Meta.MaxSkips = 3
	s.Board.Hexes = make(map[string]HexState)
	s.Board.Vertices = make(map[string]VertexState)
	s.Board.Edges = make(map[string]EdgeState)
	s.Board.Ports = make(map[string]string)
	s.Players = []Player{}
	s.Log = []LogEntry{}

	// Dev Card deck randomization
	// 14 Knights, 5 VP, 2 Road Building, 2 Year of Plenty, 2 Monopoly = 25 cards
	deck := []string{}
	for i := 0; i < 14; i++ {
		deck = append(deck, "knight")
	}
	for i := 0; i < 5; i++ {
		deck = append(deck, "vp")
	}
	deck = append(deck, "road_building", "road_building", "year_of_plenty", "year_of_plenty", "monopoly", "monopoly")
	r.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	s.Meta.DevCardDeck = deck

	// Hex randomization
	resPool := []string{"wood", "wood", "wood", "wood", "brick", "brick", "brick", "sheep", "sheep", "sheep", "sheep", "wheat", "wheat", "wheat", "wheat", "ore", "ore", "ore", "desert"}
	tokenPool := []int{2, 3, 3, 4, 4, 5, 5, 6, 6, 8, 8, 9, 9, 10, 10, 11, 11, 12}
	r.Shuffle(len(resPool), func(i, j int) { resPool[i], resPool[j] = resPool[j], resPool[i] })
	r.Shuffle(len(tokenPool), func(i, j int) { tokenPool[i], tokenPool[j] = tokenPool[j], tokenPool[i] })

	hexIDs := []string{"TB06", "TC04", "TC08", "TD02", "TD06", "TD10", "TE04", "TE08", "TF02", "TF06", "TF10", "TG04", "TG08", "TH02", "TH06", "TH10", "TI04", "TI08", "TJ06"}
	hexToVerts := map[string]string{
		"TB06": "VA06,VA07,VB05,VB08,VC06,VC07",
		"TC04": "VB04,VB05,VC03,VC06,VD04,VD05",
		"TC08": "VB08,VB09,VC07,VC10,VD08,VD09",
		"TD02": "VC02,VC03,VD01,VD04,VE02,VE03",
		"TD06": "VC06,VC07,VD05,VD08,VE06,VE07",
		"TD10": "VC10,VC11,VD09,VD12,VE10,VE11",
		"TE04": "VD04,VD05,VE03,VE06,VF04,VF05",
		"TE08": "VD08,VD09,VE07,VE10,VF08,VF09",
		"TF02": "VE02,VE03,VF01,VF04,VG02,VG03",
		"TF06": "VE06,VE07,VF05,VF08,VG06,VG07",
		"TF10": "VE10,VE11,VF09,VF12,VG10,VG11",
		"TG04": "VF04,VF05,VG03,VG06,VH04,VH05",
		"TG08": "VF08,VF09,VG07,VG10,VH08,VH09",
		"TH02": "VG02,VG03,VH01,VH04,VI02,VI03",
		"TH06": "VG06,VG07,VH05,VH08,VI06,VI07",
		"TH10": "VG10,VG11,VH09,VH12,VI10,VI11",
		"TI04": "VH04,VH05,VI03,VI06,VJ04,VJ05",
		"TI08": "VH08,VH09,VI07,VI10,VJ08,VJ09",
		"TJ06": "VI06,VI07,VJ05,VJ08,VK06,VK07",
	}

	tIdx := 0
	for i, hID := range hexIDs {
		res := resPool[i]
		token := 0
		robber := false
		if res == "desert" {
			robber = true
		} else {
			token = tokenPool[tIdx]
			tIdx++
		}
		s.Board.Hexes[hID] = HexState{
			Resource: res,
			Token:    token,
			Robber:   robber,
			Vertices: hexToVerts[hID],
		}
	}

	// Port randomization
	portIDs := []string{"P1", "P2", "P3", "P4", "P5", "P6", "P7", "P8", "P9"}
	portTypes := []string{"wood", "brick", "sheep", "wheat", "ore", "3:1", "3:1", "3:1", "3:1"}
	r.Shuffle(len(portTypes), func(i, j int) { portTypes[i], portTypes[j] = portTypes[j], portTypes[i] })
	for i, pID := range portIDs {
		s.Board.Ports[pID] = portTypes[i]
	}
}

func (s *GameState) Join(playerID string, pType string) error {
	if len(s.Players) >= 4 {
		return fmt.Errorf("game is full (max 4 players)")
	}
	for _, p := range s.Players {
		if p.ID == playerID {
			return nil // Already joined
		}
	}
	if pType == "" {
		pType = "guest"
	}
	s.Players = append(s.Players, Player{
		ID:          playerID,
		Type:        pType,
		Resources:   make(map[string]int),
		DevCards:    make(map[string]int),
		NewDevCards: make(map[string]int),
	})
	// Re-initialize resource maps to be sure
	idx := len(s.Players) - 1
	s.Players[idx].Resources = map[string]int{"wood": 0, "brick": 0, "sheep": 0, "wheat": 0, "ore": 0}
	s.Players[idx].DevCards = map[string]int{"knight": 0, "vp": 0, "road_building": 0, "year_of_plenty": 0, "monopoly": 0}
	s.Players[idx].NewDevCards = map[string]int{"knight": 0, "vp": 0, "road_building": 0, "year_of_plenty": 0, "monopoly": 0}

	s.Log = append(s.Log, LogEntry{
		Timestamp: time.Now().Unix(),
		PlayerID:  playerID,
		Action:    "join",
		Data:      pType,
	})
	return nil
}

func (s *GameState) Begin() error {
	if len(s.Players) < 3 {
		return fmt.Errorf("need 3-4 players to start")
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	order := make([]string, len(s.Players))
	for i, p := range s.Players {
		order[i] = p.ID
	}
	r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	s.Meta.Status = "setup"
	s.Meta.TurnOrder = order
	s.Meta.CurrentPlayerID = order[0]
	s.Meta.Phase = "setup_1"

	s.Log = append(s.Log, LogEntry{
		Timestamp: time.Now().Unix(),
		Action:    "begin",
		Data:      strings.Join(order, ","),
	})
	return nil
}

func (s *GameState) RemovePlayer() error {
	if len(s.Players) > 0 {
		playerID := s.Players[len(s.Players)-1].ID
		s.Players = s.Players[:len(s.Players)-1]
		s.Log = append(s.Log, LogEntry{
			Timestamp: time.Now().Unix(),
			PlayerID:  playerID,
			Action:    "remove_player",
		})
	}
	return nil
}

func (s *GameState) RecalculateVP(playerID string) {
	vp := 0
	for _, v := range s.Board.Vertices {
		if v.OwnerID == playerID {
			if v.Type == "settlement" {
				vp += 1
			} else if v.Type == "city" {
				vp += 2
			}
		}
	}
	if s.Meta.LargestArmyPlayer == playerID {
		vp += 2
	}
	if s.Meta.LongestRoadPlayer == playerID {
		vp += 2
	}
	for i, p := range s.Players {
		if p.ID == playerID {
			vp += p.DevCards["vp"]
			vp += p.NewDevCards["vp"]
			s.Players[i].VP = vp
			if vp >= 10 {
				s.Meta.Status = "finished"
			}
			break
		}
	}
}

func (s *GameState) RecalculateSpecialVP(topo *Topology) {
	// 1. Largest Army
	for _, p := range s.Players {
		if p.KnightsPlayed >= 3 && p.KnightsPlayed > s.Meta.LargestArmyCount {
			s.Meta.LargestArmyCount = p.KnightsPlayed
			s.Meta.LargestArmyPlayer = p.ID
		}
	}

	// 2. Longest Road
	for _, p := range s.Players {
		length := s.GetLongestRoad(p.ID, topo)
		if length >= 5 && length > s.Meta.LongestRoadCount {
			s.Meta.LongestRoadCount = length
			s.Meta.LongestRoadPlayer = p.ID
		}
	}

	// 3. Update everyone's VP
	for _, p := range s.Players {
		s.RecalculateVP(p.ID)
	}
}

func (s *GameState) GetLongestRoad(playerID string, topo *Topology) int {
	maxLen := 0
	playerEdges := make(map[string]bool)
	for id, e := range s.Board.Edges {
		if e.OwnerID == playerID {
			playerEdges[id] = true
		}
	}

	for eID := range playerEdges {
		length := s.dfsRoad(eID, playerID, make(map[string]bool), playerEdges, topo)
		if length > maxLen {
			maxLen = length
		}
	}
	return maxLen
}

func (s *GameState) dfsRoad(eID, playerID string, visited map[string]bool, playerEdges map[string]bool, topo *Topology) int {
	visited[eID] = true
	maxRemaining := 0

	eTopo := topo.Edges[eID]
	for _, vID := range eTopo.AdjacentVertices {
		// Road cannot pass through opponent's settlement/city
		if v, ok := s.Board.Vertices[vID]; ok && v.OwnerID != "" && v.OwnerID != "null" && v.OwnerID != playerID {
			continue
		}

		vTopo := topo.Vertices[vID]
		for _, nextEID := range vTopo.AdjacentEdges {
			if nextEID != eID && playerEdges[nextEID] && !visited[nextEID] {
				l := s.dfsRoad(nextEID, playerID, visited, playerEdges, topo)
				if l > maxRemaining {
					maxRemaining = l
				}
			}
		}
	}

	delete(visited, eID) // Backtrack
	return 1 + maxRemaining
}

func (s *GameState) EndTurn() {
	cur := s.Meta.CurrentPlayerID
	order := s.Meta.TurnOrder
	idx := -1
	for i, id := range order {
		if id == cur {
			idx = i
			break
		}
	}

	// Move new dev cards to playable hand
	for i, p := range s.Players {
		if p.ID == cur {
			for k, v := range p.NewDevCards {
				s.Players[i].DevCards[k] += v
				s.Players[i].NewDevCards[k] = 0
			}
			break
		}
	}

	s.Log = append(s.Log, LogEntry{
		Timestamp: time.Now().Unix(),
		PlayerID:  cur,
		Action:    "end_turn",
	})

	s.Meta.PendingOffers = []TradeOffer{}

	if s.Meta.Status == "setup" {
		if s.Meta.Phase == "setup_1" {
			if idx < len(order)-1 {
				s.Meta.CurrentPlayerID = order[idx+1]
			} else {
				s.Meta.Phase = "setup_2"
				// Last player goes again
			}
		} else if s.Meta.Phase == "setup_2" {
			if idx > 0 {
				s.Meta.CurrentPlayerID = order[idx-1]
			} else {
				s.Meta.Status = "active"
				s.Meta.Phase = "roll"
			}
		}
	} else {
		nextIdx := (idx + 1) % len(order)
		s.Meta.CurrentPlayerID = order[nextIdx]
		s.Meta.Phase = "roll"
	}
	s.Meta.LastActionTimestamp = time.Now().Unix()
}

func (s *GameState) Roll(forced int, topo *Topology) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	d1, d2 := r.Intn(6)+1, r.Intn(6)+1
	s.Meta.LastRoll1 = d1
	s.Meta.LastRoll2 = d2
	total := d1 + d2
	if forced > 0 {
		total = forced
	}

	s.Log = append(s.Log, LogEntry{
		Timestamp: time.Now().Unix(),
		PlayerID:  s.Meta.CurrentPlayerID,
		Action:    "roll",
		Data:      fmt.Sprintf("%d", total),
	})

	if total == 7 {
		// Identify players who must discard
		s.Meta.DiscardingPlayers = []string{}
		for _, p := range s.Players {
			totalRes := 0
			for _, count := range p.Resources {
				totalRes += count
			}
			if totalRes > 7 {
				s.Meta.DiscardingPlayers = append(s.Meta.DiscardingPlayers, p.ID)
			}
		}

		if len(s.Meta.DiscardingPlayers) > 0 {
			s.Meta.Phase = "robber_discard"
		} else {
			s.Meta.Phase = "robber_move"
		}
	} else {
		s.Meta.Phase = "action"
		// Resource production
		demand := make(map[string]map[string]int) // resType -> playerID -> count

		for _, h := range s.Board.Hexes {
			if h.Token == total && !h.Robber {
				verts := strings.Split(h.Vertices, ",")
				for _, vID := range verts {
					if v, ok := s.Board.Vertices[vID]; ok && v.OwnerID != "" && v.OwnerID != "null" {
						amount := 1
						if v.Type == "city" {
							amount = 2
						}
						if demand[h.Resource] == nil {
							demand[h.Resource] = make(map[string]int)
						}
						demand[h.Resource][v.OwnerID] += amount
					}
				}
			}
		}

		// Distribute resources if bank can satisfy full demand per resource type
		for resType, players := range demand {
			totalDemand := 0
			for _, count := range players {
				totalDemand += count
			}

			currentInHands := s.GetTotalResources(resType)
			availableInBank := MaxBank - currentInHands

			if totalDemand <= availableInBank {
				for pID, count := range players {
					for i, p := range s.Players {
						if p.ID == pID {
							s.Players[i].Resources[resType] += count
							break
						}
					}
				}
			}
		}
	}
	s.Meta.LastActionTimestamp = time.Now().Unix()
}

func (s *GameState) Move(playerID, moveType, target string, topo *Topology) error {
	// Rule Enforcement
	if moveType != "cheat_resources" && moveType != "trade_bank" && moveType != "trade_port" && moveType != "buy_dev_card" && moveType != "play_dev_card" && moveType != "discard" && moveType != "move_robber" && moveType != "steal_resource" {
		if err := s.validateMoveLocal(playerID, moveType, target, topo); err != nil {
			return err
		}
	}

	// Apply Move
	switch moveType {
	case "build_settlement":
		s.Board.Vertices[target] = VertexState{OwnerID: playerID, Type: "settlement"}
		s.RecalculateSpecialVP(topo)
	case "build_city":
		s.Board.Vertices[target] = VertexState{OwnerID: playerID, Type: "city"}
		s.RecalculateVP(playerID)
	case "build_road":
		s.Board.Edges[target] = EdgeState{OwnerID: playerID}
		s.RecalculateSpecialVP(topo)
	case "discard":
		return s.Discard(playerID, target)
	case "move_robber":
		return s.MoveRobber(playerID, target, topo)
	case "steal_resource":
		return s.StealResource(playerID, target)
	case "cheat_resources":
		for i, p := range s.Players {
			if p.ID == playerID {
				for res := range p.Resources {
					available := MaxBank - s.GetTotalResources(res)
					if available > 0 {
						amount := 10
						if amount > available {
							amount = available
						}
						s.Players[i].Resources[res] += amount
					}
				}
				break
			}
		}
	case "buy_dev_card":
		return s.BuyDevCard(playerID)
	case "play_dev_card":
		// target: card_type:extra_data
		parts := strings.Split(target, ":")
		card := parts[0]
		extra := ""
		if len(parts) > 1 {
			extra = parts[1]
		}
		return s.PlayDevCard(playerID, card, extra, topo)
	case "trade_bank":
		// target: give:get
		parts := strings.Split(target, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid trade data")
		}
		return s.TradeBank(playerID, parts[0], parts[1])
	case "trade_port":
		// target: give:get
		parts := strings.Split(target, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid trade data")
		}
		return s.TradePort(playerID, parts[0], parts[1], topo)
	case "submit_trade_offer":
		// target: give:get:countGive:countGet
		parts := strings.Split(target, ":")
		if len(parts) != 4 {
			return fmt.Errorf("invalid offer data")
		}
		give, get := parts[0], parts[1]
		var countGive, countGet int
		fmt.Sscanf(parts[2], "%d", &countGive)
		fmt.Sscanf(parts[3], "%d", &countGet)

		offer := TradeOffer{
			ID:           fmt.Sprintf("T%d", time.Now().UnixNano()),
			FromPlayerID: playerID,
			GiveResource: give,
			GiveAmount:   countGive,
			GetResource:  get,
			GetAmount:    countGet,
		}
		s.Meta.PendingOffers = append(s.Meta.PendingOffers, offer)
		return nil
	case "accept_trade_offer":
		// target: offer_id
		for idx, offer := range s.Meta.PendingOffers {
			if offer.ID == target {
				// FromPlayer gives GiveResource, ActivePlayer gives GetResource
				activePID := s.Meta.CurrentPlayerID
				var fromPlayer, activePlayer *Player
				for i := range s.Players {
					if s.Players[i].ID == offer.FromPlayerID {
						fromPlayer = &s.Players[i]
					}
					if s.Players[i].ID == activePID {
						activePlayer = &s.Players[i]
					}
				}
				if fromPlayer == nil || activePlayer == nil {
					return fmt.Errorf("player not found")
				}
				if fromPlayer.Resources[offer.GiveResource] < offer.GiveAmount {
					return fmt.Errorf("offering player insufficient resources")
				}
				if activePlayer.Resources[offer.GetResource] < offer.GetAmount {
					return fmt.Errorf("you have insufficient resources")
				}

				// Execute Trade
				fromPlayer.Resources[offer.GiveResource] -= offer.GiveAmount
				fromPlayer.Resources[offer.GetResource] += offer.GetAmount
				activePlayer.Resources[offer.GetResource] -= offer.GetAmount
				activePlayer.Resources[offer.GiveResource] += offer.GiveAmount

				// Remove offer
				s.Meta.PendingOffers = append(s.Meta.PendingOffers[:idx], s.Meta.PendingOffers[idx+1:]...)
				return nil
			}
		}
		return fmt.Errorf("offer not found")
	case "reject_trade_offer":
		// target: offer_id
		for idx, offer := range s.Meta.PendingOffers {
			if offer.ID == target {
				s.Meta.PendingOffers = append(s.Meta.PendingOffers[:idx], s.Meta.PendingOffers[idx+1:]...)
				return nil
			}
		}
		return fmt.Errorf("offer not found")
	}

	s.Log = append(s.Log, LogEntry{
		Timestamp: time.Now().Unix(),
		PlayerID:  playerID,
		Action:    moveType,
		Data:      target,
	})
	s.Meta.LastActionTimestamp = time.Now().Unix()
	return nil
}

func (s *GameState) Discard(playerID, target string) error {
	if s.Meta.Phase != "robber_discard" {
		return fmt.Errorf("not in discard phase")
	}
	found := false
	for i, id := range s.Meta.DiscardingPlayers {
		if id == playerID {
			s.Meta.DiscardingPlayers = append(s.Meta.DiscardingPlayers[:i], s.Meta.DiscardingPlayers[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("you don't need to discard")
	}

	s.Log = append(s.Log, LogEntry{
		Timestamp: time.Now().Unix(),
		PlayerID:  playerID,
		Action:    "discard",
		Data:      target,
	})

	// Parse target: "wood:1,brick:2"
	parts := strings.Split(target, ",")
	for _, p := range parts {
		kv := strings.Split(p, ":")
		if len(kv) != 2 {
			continue
		}
		res := kv[0]
		var count int
		fmt.Sscanf(kv[1], "%d", &count)

		for i, p := range s.Players {
			if p.ID == playerID {
				if s.Players[i].Resources[res] < count {
					return fmt.Errorf("insufficient %s", res)
				}
				s.Players[i].Resources[res] -= count
				break
			}
		}
	}

	if len(s.Meta.DiscardingPlayers) == 0 {
		s.Meta.Phase = "robber_move"
	}
	return nil
}

func (s *GameState) MoveRobber(playerID, hexID string, topo *Topology) error {
	if playerID != s.Meta.CurrentPlayerID {
		return fmt.Errorf("not your turn")
	}
	if s.Meta.Phase != "robber_move" {
		return fmt.Errorf("not in robber_move phase")
	}

	oldHexID := ""
	for id, h := range s.Board.Hexes {
		if h.Robber {
			oldHexID = id
			break
		}
	}
	if hexID == oldHexID {
		return fmt.Errorf("must move robber to a new hex")
	}

	h, ok := s.Board.Hexes[hexID]
	if !ok {
		return fmt.Errorf("invalid hex")
	}

	// Update Robber
	for id := range s.Board.Hexes {
		h := s.Board.Hexes[id]
		h.Robber = (id == hexID)
		s.Board.Hexes[id] = h
	}

	// Identify potential victims
	victims := make(map[string]bool)
	verts := strings.Split(h.Vertices, ",")
	for _, vID := range verts {
		if v, ok := s.Board.Vertices[vID]; ok && v.OwnerID != "" && v.OwnerID != "null" && v.OwnerID != playerID {
			// Check if they have resources
			for _, p := range s.Players {
				if p.ID == v.OwnerID {
					total := 0
					for _, c := range p.Resources {
						total += c
					}
					if total > 0 {
						victims[v.OwnerID] = true
					}
					break
				}
			}
		}
	}

	if len(victims) == 0 {
		s.Meta.Phase = "action"
	} else {
		s.Meta.Phase = "robber_steal"
	}

	return nil
}

func (s *GameState) StealResource(playerID, victimID string) error {
	if playerID != s.Meta.CurrentPlayerID {
		return fmt.Errorf("not your turn")
	}
	if s.Meta.Phase != "robber_steal" {
		return fmt.Errorf("not in robber_steal phase")
	}

	var thief, victim *Player
	for i := range s.Players {
		if s.Players[i].ID == playerID {
			thief = &s.Players[i]
		}
		if s.Players[i].ID == victimID {
			victim = &s.Players[i]
		}
	}

	if thief == nil || victim == nil {
		return fmt.Errorf("player not found")
	}

	// Pick random resource from victim
	resPool := []string{}
	for res, count := range victim.Resources {
		for i := 0; i < count; i++ {
			resPool = append(resPool, res)
		}
	}

	if len(resPool) == 0 {
		s.Meta.Phase = "action"
		return nil
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	stolenRes := resPool[r.Intn(len(resPool))]

	victim.Resources[stolenRes]--
	thief.Resources[stolenRes]++

	s.Meta.Phase = "action"
	return nil
}

func (s *GameState) BuyDevCard(playerID string) error {
	if len(s.Meta.DevCardDeck) == 0 {
		return fmt.Errorf("deck empty")
	}
	for i, p := range s.Players {
		if p.ID == playerID {
			for res, amount := range BuildCosts["dev_card"] {
				if p.Resources[res] < amount {
					return fmt.Errorf("insufficient %s", res)
				}
			}
			for res, amount := range BuildCosts["dev_card"] {
				s.Players[i].Resources[res] -= amount
			}
			card := s.Meta.DevCardDeck[0]
			s.Meta.DevCardDeck = s.Meta.DevCardDeck[1:]
			s.Players[i].NewDevCards[card]++
			if card == "vp" {
				s.RecalculateVP(playerID)
			}
			return nil
		}
	}
	return fmt.Errorf("player not found")
}

func (s *GameState) PlayDevCard(playerID, card, extra string, topo *Topology) error {
	for i, p := range s.Players {
		if p.ID == playerID {
			if p.DevCards[card] <= 0 {
				return fmt.Errorf("no %s available to play", card)
			}
			s.Players[i].DevCards[card]--

			switch card {
			case "knight":
				s.Players[i].KnightsPlayed++
				s.RecalculateSpecialVP(topo)
				s.Meta.Phase = "robber_move"
			case "road_building":
				// Normally grants 2 roads. For simplicity, just add resources for 2 roads.
				s.Players[i].Resources["wood"] += 2
				s.Players[i].Resources["brick"] += 2
			case "year_of_plenty":
				// extra: res1,res2
				resParts := strings.Split(extra, ",")
				for _, r := range resParts {
					if _, ok := s.Players[i].Resources[r]; ok {
						s.Players[i].Resources[r]++
					}
				}
			case "monopoly":
				// extra: resource
				for j, other := range s.Players {
					if other.ID != playerID {
						count := other.Resources[extra]
						s.Players[j].Resources[extra] = 0
						s.Players[i].Resources[extra] += count
					}
				}
			}
			return nil
		}
	}
	return fmt.Errorf("player not found")
}

func (s *GameState) TradeBank(playerID, give, get string) error {
	for i, p := range s.Players {
		if p.ID == playerID {
			if p.Resources[give] < 4 {
				return fmt.Errorf("not enough %s (need 4)", give)
			}
			if s.GetTotalResources(get)+1 > MaxBank {
				return fmt.Errorf("bank empty of %s", get)
			}
			s.Players[i].Resources[give] -= 4
			s.Players[i].Resources[get] += 1
			return nil
		}
	}
	return fmt.Errorf("player not found")
}

func (s *GameState) TradePort(playerID, give, get string, topo *Topology) error {
	ports := s.GetPlayerPorts(playerID, topo)
	rate := 0
	for _, p := range ports {
		if p == give {
			rate = 2
			break
		}
		if p == "3:1" && rate == 0 {
			rate = 3
		}
	}
	if rate == 0 {
		return fmt.Errorf("no applicable port")
	}

	for i, p := range s.Players {
		if p.ID == playerID {
			if p.Resources[give] < rate {
				return fmt.Errorf("not enough %s (need %d)", give, rate)
			}
			if s.GetTotalResources(get)+1 > MaxBank {
				return fmt.Errorf("bank empty of %s", get)
			}
			s.Players[i].Resources[give] -= rate
			s.Players[i].Resources[get] += 1
			return nil
		}
	}
	return fmt.Errorf("player not found")
}

func (s *GameState) GetPlayerPorts(playerID string, topo *Topology) []string {
	var ports []string
	for vID, v := range s.Board.Vertices {
		if v.OwnerID == playerID {
			if vTopo, ok := topo.Vertices[vID]; ok && vTopo.Port != "" {
				ports = append(ports, vTopo.Port)
			}
		}
	}
	return ports
}

func (s GameState) DeepCopy() GameState {
	res := GameState{
		Meta: Meta{
			Status:              s.Meta.Status,
			LastActionTimestamp: s.Meta.LastActionTimestamp,
			TimeoutSeconds:      s.Meta.TimeoutSeconds,
			MaxSkips:            s.Meta.MaxSkips,
			TurnOrder:           append([]string(nil), s.Meta.TurnOrder...),
			CurrentPlayerID:     s.Meta.CurrentPlayerID,
			Phase:               s.Meta.Phase,
			DevCardDeck:         append([]string(nil), s.Meta.DevCardDeck...),
			DiscardingPlayers:   append([]string(nil), s.Meta.DiscardingPlayers...),
			LargestArmyPlayer:   s.Meta.LargestArmyPlayer,
			LargestArmyCount:    s.Meta.LargestArmyCount,
			LongestRoadPlayer:   s.Meta.LongestRoadPlayer,
			LongestRoadCount:    s.Meta.LongestRoadCount,
			LastRoll1:           s.Meta.LastRoll1,
			LastRoll2:           s.Meta.LastRoll2,
		},
		Board: BoardState{
			Hexes:    make(map[string]HexState),
			Vertices: make(map[string]VertexState),
			Edges:    make(map[string]EdgeState),
			Ports:    make(map[string]string),
		},
		Players: make([]Player, len(s.Players)),
		Log:     make([]LogEntry, len(s.Log)),
	}

	for k, v := range s.Board.Hexes {
		res.Board.Hexes[k] = v
	}
	for k, v := range s.Board.Vertices {
		res.Board.Vertices[k] = v
	}
	for k, v := range s.Board.Edges {
		res.Board.Edges[k] = v
	}
	for k, v := range s.Board.Ports {
		res.Board.Ports[k] = v
	}

	for i, p := range s.Players {
		newP := Player{
			ID:            p.ID,
			Type:          p.Type,
			Resources:     make(map[string]int),
			VP:            p.VP,
			SkipCount:     p.SkipCount,
			DevCards:      make(map[string]int),
			NewDevCards:   make(map[string]int),
			KnightsPlayed: p.KnightsPlayed,
		}
		for k, v := range p.Resources {
			newP.Resources[k] = v
		}
		for k, v := range p.DevCards {
			newP.DevCards[k] = v
		}
		for k, v := range p.NewDevCards {
			newP.NewDevCards[k] = v
		}
		res.Players[i] = newP
	}

	copy(res.Log, s.Log)
	return res
}

func (s *GameState) Replay(topo *Topology) []GameState {
	var history []GameState

	// Initial state with board and meta needed for replay
	current := GameState{
		Board: BoardState{
			Hexes:    make(map[string]HexState),
			Vertices: make(map[string]VertexState),
			Edges:    make(map[string]EdgeState),
			Ports:    make(map[string]string),
		},
		Meta: Meta{
			Status: "invitation",
			// We might need to restore DevCardDeck if it was randomized at Init
			DevCardDeck: s.Meta.DevCardDeck,
		},
		Players: []Player{},
		Log:     []LogEntry{},
	}
	// Copy board
	for k, v := range s.Board.Hexes {
		current.Board.Hexes[k] = v
	}
	for k, v := range s.Board.Vertices {
		// Reset vertex ownership for replay
		v.OwnerID = ""
		v.Type = ""
		current.Board.Vertices[k] = v
	}
	for k, v := range s.Board.Edges {
		// Reset edge ownership for replay
		v.OwnerID = ""
		current.Board.Edges[k] = v
	}
	for k, v := range s.Board.Ports {
		current.Board.Ports[k] = v
	}

	history = append(history, current.DeepCopy())

	for _, entry := range s.Log {
		switch entry.Action {
		case "join":
			current.Join(entry.PlayerID, entry.Data)
		case "begin":
			current.Begin()
			if entry.Data != "" {
				current.Meta.TurnOrder = strings.Split(entry.Data, ",")
				current.Meta.CurrentPlayerID = current.Meta.TurnOrder[0]
			}
		case "roll":
			var forced int
			fmt.Sscanf(entry.Data, "%d", &forced)
			current.Roll(forced, topo)
		case "move_robber", "build_settlement", "build_road", "build_city", "steal_resource", "play_dev_card", "buy_dev_card", "trade_bank", "cheat_resources":
			current.Move(entry.PlayerID, entry.Action, entry.Data, topo)
		case "end_turn":
			current.EndTurn()
		case "discard":
			current.Discard(entry.PlayerID, entry.Data)
		case "remove_player":
			current.RemovePlayer()
		}
		// Clear log in history frames to save space/context
		current.Log = []LogEntry{}
		history = append(history, current.DeepCopy())
	}
	return history
}

func (s *GameState) Simulate(topo *Topology) []GameState {
	var history []GameState
	
	s.Init(topo)
	history = append(history, s.DeepCopy())

	s.Join("bot1", "bot")
	s.Join("bot2", "bot")
	s.Join("bot3", "bot")
	history = append(history, s.DeepCopy())

	s.Begin()
	history = append(history, s.DeepCopy())

	maxTurns := 500
	turnCount := 0

	for s.Meta.Status != "finished" && turnCount < maxTurns {
		pid := s.Meta.CurrentPlayerID
		var p Player
		for _, pl := range s.Players {
			if pl.ID == pid {
				p = pl
				break
			}
		}

		if s.Meta.Phase == "roll" {
			s.Roll(0, topo)
			history = append(history, s.DeepCopy())
		}

		// Handle Robber Phases
		if s.Meta.Phase == "robber_discard" {
			dpCopy := make([]string, len(s.Meta.DiscardingPlayers))
			copy(dpCopy, s.Meta.DiscardingPlayers)
			for _, dpID := range dpCopy {
				var p Player
				for _, pl := range s.Players {
					if pl.ID == dpID {
						p = pl
						break
					}
				}
				total := 0
				for _, c := range p.Resources {
					total += c
				}
				toDiscard := total / 2
				discardStr := []string{}
				resOrder := []string{"sheep", "wood", "brick", "wheat", "ore"} // Discard less valuable first
				for _, res := range resOrder {
					count := p.Resources[res]
					if toDiscard <= 0 {
						break
					}
					num := count
					if num > toDiscard {
						num = toDiscard
					}
					if num > 0 {
						discardStr = append(discardStr, fmt.Sprintf("%s:%d", res, num))
						toDiscard -= num
					}
				}
				if len(discardStr) > 0 {
					s.Move(dpID, "discard", strings.Join(discardStr, ","), topo)
					history = append(history, s.DeepCopy())
				}
			}
		}

		if s.Meta.Phase == "robber_move" {
			bestHex := ""
			maxValue := -1
			for hID, h := range s.Board.Hexes {
				if h.Robber || h.Resource == "desert" {
					continue
				}
				ours := false
				enemyValue := 0
				verts := strings.Split(h.Vertices, ",")
				for _, vID := range verts {
					if v, ok := s.Board.Vertices[vID]; ok && v.OwnerID != "" && v.OwnerID != "null" {
						if v.OwnerID == pid {
							ours = true
							break
						}
						val := 1
						if v.Type == "city" {
							val = 2
						}
						enemyValue += val
					}
				}
				if ours {
					continue
				}
				// Heuristic: enemy settlements * probability of token
				prob := 6 - int(math.Abs(float64(h.Token-7)))
				if enemyValue*prob > maxValue {
					maxValue = enemyValue * prob
					bestHex = hID
				}
			}
			if bestHex == "" {
				for hID := range s.Board.Hexes {
					if !s.Board.Hexes[hID].Robber {
						bestHex = hID
						break
					}
				}
			}
			s.Move(pid, "move_robber", bestHex, topo)
			history = append(history, s.DeepCopy())
		}

		if s.Meta.Phase == "robber_steal" {
			var robberHex HexState
			for _, h := range s.Board.Hexes {
				if h.Robber {
					robberHex = h
					break
				}
			}
			victimID := ""
			verts := strings.Split(robberHex.Vertices, ",")
			for _, vID := range verts {
				if v, ok := s.Board.Vertices[vID]; ok && v.OwnerID != "" && v.OwnerID != "null" && v.OwnerID != pid {
					victimID = v.OwnerID
					// Prefer stealing from player with most VP
					break
				}
			}
			if victimID != "" {
				s.Move(pid, "steal_resource", victimID, topo)
			} else {
				s.Meta.Phase = "action"
			}
			history = append(history, s.DeepCopy())
		}

		// AI Logic: Cheat resources to ensure progress for mechanics test (only in action phase)
		if s.Meta.Phase == "action" {
			s.Move(pid, "cheat_resources", "", topo)
		}

		if (s.Meta.Phase == "setup_1" || s.Meta.Phase == "setup_2") && s.Meta.Status != "finished" {
			// In setup, must build 1 settlement AND 2 roads
			builtSettlement := false
			for vID := range topo.Vertices {
				if err := s.validateMoveLocal(pid, "build_settlement", vID, topo); err == nil {
					s.Move(pid, "build_settlement", vID, topo)
					builtSettlement = true
					// If setup_2, give resources for adjacent hexes
					if s.Meta.Phase == "setup_2" {
						for _, h := range s.Board.Hexes {
							if strings.Contains(h.Vertices, vID) && h.Resource != "desert" {
								if s.GetTotalResources(h.Resource) < MaxBank {
									for i, pl := range s.Players {
										if pl.ID == pid {
											s.Players[i].Resources[h.Resource]++
											break
										}
									}
								}
							}
						}
					}
					break
				}
			}
			if builtSettlement {
				for i := 0; i < 2; i++ {
					builtRoad := false
					for eID := range topo.Edges {
						if err := s.validateMoveLocal(pid, "build_road", eID, topo); err == nil {
							s.Move(pid, "build_road", eID, topo)
							builtRoad = true
							break
						}
					}
					if !builtRoad {
						// Not enough space for 2 roads, break
						break
					}
				}
			}
			history = append(history, s.DeepCopy())
		} else if s.Meta.Phase == "action" && s.Meta.Status != "finished" {
			// Try to play a knight if we have one
			if p.DevCards["knight"] > 0 {
				s.Move(pid, "play_dev_card", "knight", topo)
			}
			
			if s.Meta.Status == "finished" {
				history = append(history, s.DeepCopy())
			} else {
				// Try to buy a dev card if we have extra wheat/sheep/ore
				s.Move(pid, "buy_dev_card", "", topo)
				
				if s.Meta.Status == "finished" {
					history = append(history, s.DeepCopy())
				} else {
					// Try to trade if we have too much of one resource
					for _, res := range []string{"wood", "brick", "sheep", "wheat", "ore"} {
						if s.Meta.Status == "finished" {
							break
						}
						var player Player
						found := false
						for _, pl := range s.Players {
							if pl.ID == pid {
								player = pl
								found = true
								break
							}
						}
						if found && player.Resources[res] >= 4 {
							// Trade for something we have 0 of
							for _, other := range []string{"wood", "brick", "sheep", "wheat", "ore"} {
								if player.Resources[other] == 0 {
									s.Move(pid, "trade_bank", res+":"+other, topo)
									break
								}
							}
						}
					}

					// Try to build multiple things if possible
					for i := 0; i < 3; i++ {
						if s.Meta.Status == "finished" {
							break
						}
						built := false
						// 1. Try to build settlement
						for vID := range topo.Vertices {
							if err := s.validateMoveLocal(pid, "build_settlement", vID, topo); err == nil {
								s.Move(pid, "build_settlement", vID, topo)
								history = append(history, s.DeepCopy())
								built = true
								break
							}
						}
						if s.Meta.Status == "finished" {
							break
						}
						// 2. Try to build road
						if !built {
							for eID := range topo.Edges {
								if err := s.validateMoveLocal(pid, "build_road", eID, topo); err == nil {
									s.Move(pid, "build_road", eID, topo)
									history = append(history, s.DeepCopy())
									built = true
									break
								}
							}
						}
						if s.Meta.Status == "finished" {
							break
						}
						// 3. Try to build city
						if !built {
							for vID, v := range s.Board.Vertices {
								if v.OwnerID == pid && v.Type == "settlement" {
									if err := s.validateMoveLocal(pid, "build_city", vID, topo); err == nil {
										s.Move(pid, "build_city", vID, topo)
										history = append(history, s.DeepCopy())
										built = true
										break
									}
								}
							}
						}
						if !built {
							break
						}
					}
				}
			}
		}

		if s.Meta.Status == "finished" {
			break
		}

		s.EndTurn()
		history = append(history, s.DeepCopy())
		turnCount++
	}

	return history
}

func (s *GameState) GetPieceCounts(playerID string) (roads, settlements, cities int) {
	for _, v := range s.Board.Vertices {
		if v.OwnerID == playerID {
			if v.Type == "settlement" {
				settlements++
			} else if v.Type == "city" {
				cities++
			}
		}
	}
	for _, e := range s.Board.Edges {
		if e.OwnerID == playerID {
			roads++
		}
	}
	return
}

func (s *GameState) GetTotalResources(resType string) int {
	total := 0
	for _, p := range s.Players {
		total += p.Resources[resType]
	}
	return total
}

func (s *GameState) GetRollResources(roll int) map[string]map[string]int {
	demand := make(map[string]map[string]int) // resType -> playerID -> count
	if roll == 7 || roll <= 0 {
		return demand
	}

	for _, h := range s.Board.Hexes {
		if h.Token == roll && !h.Robber {
			verts := strings.Split(h.Vertices, ",")
			for _, vID := range verts {
				if v, ok := s.Board.Vertices[vID]; ok && v.OwnerID != "" && v.OwnerID != "null" {
					amount := 1
					if v.Type == "city" {
						amount = 2
					}
					if demand[h.Resource] == nil {
						demand[h.Resource] = make(map[string]int)
					}
					demand[h.Resource][v.OwnerID] += amount
				}
			}
		}
	}
	return demand
}

// Add a non-method validation helper for simulation
func (s *GameState) validateMoveLocal(playerID, moveType, target string, topo *Topology) error {
	roads, settlements, cities := s.GetPieceCounts(playerID)

	switch moveType {
	case "build_settlement":
		if s.Meta.Phase == "setup_1" && settlements >= 1 {
			return fmt.Errorf("only 1 settlement allowed in setup 1")
		}
		if s.Meta.Phase == "setup_2" && settlements >= 2 {
			return fmt.Errorf("only 2 settlements allowed in setup")
		}
		if settlements >= MaxSettlements {
			return fmt.Errorf("no more settlements available")
		}
		if v, ok := s.Board.Vertices[target]; ok && v.OwnerID != "" && v.OwnerID != "null" {
			return fmt.Errorf("occupied")
		}
		// Distance Rule
		vTopo := topo.Vertices[target]
		for _, eID := range vTopo.AdjacentEdges {
			edgeTopo := topo.Edges[eID]
			for _, vID := range edgeTopo.AdjacentVertices {
				if vID == target {
					continue
				}
				if neighbor, ok := s.Board.Vertices[vID]; ok && neighbor.OwnerID != "" && neighbor.OwnerID != "null" {
					return fmt.Errorf("distance")
				}
			}
		}
		// Connectivity
		if s.Meta.Status != "setup" {
			connected := false
			for _, eID := range vTopo.AdjacentEdges {
				if e, ok := s.Board.Edges[eID]; ok && e.OwnerID == playerID {
					connected = true
					break
				}
			}
			if !connected {
				return fmt.Errorf("connectivity")
			}
		}
	case "build_road":
		if s.Meta.Phase == "setup_1" && roads >= 2 {
			return fmt.Errorf("only 2 roads allowed in setup 1")
		}
		if s.Meta.Phase == "setup_2" && roads >= 4 {
			return fmt.Errorf("only 4 roads allowed in setup")
		}
		if roads >= MaxRoads {
			return fmt.Errorf("no more roads available")
		}
		if e, ok := s.Board.Edges[target]; ok && e.OwnerID != "" && e.OwnerID != "null" {
			return fmt.Errorf("occupied")
		}
		eTopo := topo.Edges[target]
		connected := false
		for _, vID := range eTopo.AdjacentVertices {
			if v, ok := s.Board.Vertices[vID]; ok && v.OwnerID == playerID {
				connected = true
				break
			}
			vTopo := topo.Vertices[vID]
			for _, adjEID := range vTopo.AdjacentEdges {
				if adjEID == target {
					continue
				}
				if e, ok := s.Board.Edges[adjEID]; ok && e.OwnerID == playerID {
					connected = true
					break
				}
			}
			if connected {
				break
			}
		}
		if !connected {
			return fmt.Errorf("connectivity")
		}
	case "build_city":
		if cities >= MaxCities {
			return fmt.Errorf("no more cities available")
		}
		if v, ok := s.Board.Vertices[target]; !ok || v.OwnerID != playerID || v.Type != "settlement" {
			return fmt.Errorf("must upgrade an owned settlement")
		}
	}
	return nil
}

// Topology structs (matching catan-cli/topology.yaml)
type Topology struct {
	Edges    map[string]EdgeTopology   `yaml:"edges"`
	Vertices map[string]VertexTopology `yaml:"vertices"`
	Ports    map[string]PortTopology   `yaml:"ports"`
}

type EdgeTopology struct {
	AdjacentVertices []string `yaml:"adjacent_vertices"`
	X                int      `yaml:"x"`
	Y                int      `yaml:"y"`
}

type VertexTopology struct {
	AdjacentEdges []string `yaml:"adjacent_edges"`
	X             int      `yaml:"x"`
	Y             int      `yaml:"y"`
	Port          string   `yaml:"-"` // e.g. "3:1" or "brick"
}

type PortTopology struct {
	Type     string   `yaml:"type"`
	Vertices []string `yaml:"vertices"`
}

type model struct {
	board        string
	state        GameState
	topology     Topology
	playerStyles map[string]lipgloss.Style
	selectedType string // "vertex" or "edge"
	selectedID   string
	message      string
	width        int
	height       int
	joinType     int // 0: guest, 1: git, 2: bot
	history      []GameState
	historyIdx   int
	isPlaying    bool
	viewMode     int // 0: Board, 1: Hand/Trading
	isSimulating bool
	isPrompting  bool
	inputText    string
	tradeStep    int    // 0: Select Give, 1: Select Get, 2: Select Target (Bank/Port/Player)
	tradeGive    string
	tradeGet     string
	tradeCursor  int
	offerIdx     int
}

type simulationMsg []GameState

type gitUserCheckMsg struct {
	Username string
	Exists   bool
	Error    string
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func initialModel() model {
	boardData, err := os.ReadFile("board.txt")
	if err != nil {
		panic(err)
	}

	gameData, err := os.ReadFile("game.yaml")
	if err != nil {
		panic(err)
	}

	var state GameState
	if err := yaml.Unmarshal(gameData, &state); err != nil {
		panic(err)
	}

	topologyData, err := os.ReadFile("topology.yaml")
	if err != nil {
		panic(err)
	}

	var topology Topology
	if err := yaml.Unmarshal(topologyData, &topology); err != nil {
		panic(err)
	}

	m := model{
		board:    string(boardData),
		state:    state,
		topology: topology,
		width:    120,
		height:   60,
	}
	m.relinkPorts()

	playerStyles := make(map[string]lipgloss.Style)
	for i, p := range state.Players {
		playerStyles[p.ID] = playerColorStyles[i%len(playerColorStyles)]
	}

	// Default cursor to the first vertex
	var firstVertex string
	for id := range m.topology.Vertices {
		firstVertex = id
		break
	}

	m.playerStyles = playerStyles
	m.selectedType = "vertex"
	m.selectedID = firstVertex
	return m
}

func (m *model) relinkPorts() {
	// 1. Reset all vertices to no port
	for id, v := range m.topology.Vertices {
		v.Port = ""
		m.topology.Vertices[id] = v
	}

	// 2. Map default ports from topology
	for _, port := range m.topology.Ports {
		for _, vID := range port.Vertices {
			if v, ok := m.topology.Vertices[vID]; ok {
				v.Port = port.Type
				m.topology.Vertices[vID] = v
			}
		}
	}

	// 3. Override with randomized ports from game state if they exist
	for pID, pType := range m.state.Board.Ports {
		if port, ok := m.topology.Ports[pID]; ok {
			for _, vID := range port.Vertices {
				if v, ok := m.topology.Vertices[vID]; ok {
					v.Port = pType
					m.topology.Vertices[vID] = v
				}
			}
		}
	}
}

func checkGitHubUser(username string) tea.Cmd {
	return func() tea.Msg {
		token := os.Getenv("GIT_TOKEN")
		if token == "" {
			return gitUserCheckMsg{Username: username, Exists: false, Error: "GIT_TOKEN not set"}
		}

		client := &http.Client{Timeout: 5 * time.Second}
		req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/users/%s", username), nil)
		req.Header.Set("Authorization", "token "+token)

		resp, err := client.Do(req)
		if err != nil {
			return gitUserCheckMsg{Username: username, Exists: false, Error: err.Error()}
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			return gitUserCheckMsg{Username: username, Exists: true}
		} else if resp.StatusCode == 404 {
			return gitUserCheckMsg{Username: username, Exists: false, Error: "GitHub user not found"}
		} else {
			return gitUserCheckMsg{Username: username, Exists: false, Error: fmt.Sprintf("GitHub API error: %d", resp.StatusCode)}
		}
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case simulationMsg:
		m.isSimulating = false
		m.history = msg
		m.historyIdx = 0
		m.isPlaying = true
		m.state = m.history[0]
		m.relinkPorts()
		return m, tick()

	case gitUserCheckMsg:
		if msg.Exists {
			m.runDM("join", msg.Username, "git")
			m.message = fmt.Sprintf("GitHub user %s joined.", msg.Username)
		} else {
			m.message = fmt.Sprintf("Error: %s", msg.Error)
		}
		return m, nil

	case tickMsg:
		if m.isPlaying && m.historyIdx < len(m.history)-1 {
			m.historyIdx++
			m.state = m.history[m.historyIdx]
			m.relinkPorts()
			return m, tick()
		}
		m.isPlaying = false
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.isPlaying || len(m.history) > 0 {
			switch msg.String() {
			case " ":
				m.isPlaying = !m.isPlaying
				if m.isPlaying {
					return m, tick()
				}
				return m, nil
			case "left":
				if m.historyIdx > 0 {
					m.isPlaying = false
					m.historyIdx--
					m.state = m.history[m.historyIdx]
					m.relinkPorts()
				}
				return m, nil
			case "right":
				if m.historyIdx < len(m.history)-1 {
					m.isPlaying = false
					m.historyIdx++
					m.state = m.history[m.historyIdx]
					m.relinkPorts()
				}
				return m, nil
			}
		}

		if m.isPrompting {
			switch msg.String() {
			case "enter":
				m.isPrompting = false
				if m.inputText != "" {
					username := m.inputText
					m.inputText = ""
					m.message = fmt.Sprintf("Checking GitHub user %s...", username)
					return m, checkGitHubUser(username)
				}
				m.inputText = ""
				return m, nil
			case "esc":
				m.isPrompting = false
				m.inputText = ""
				return m, nil
			case "backspace":
				if len(m.inputText) > 0 {
					m.inputText = m.inputText[:len(m.inputText)-1]
				}
				return m, nil
			default:
				s := msg.String()
				if len(s) == 1 && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= '0' && s[0] <= '9') || s[0] == '-' || s[0] == '_') {
					m.inputText += s
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "i":
			m.runDM("init")
		case "p":
			if m.isSimulating {
				return m, nil
			}
			m.isSimulating = true
			m.message = "Simulating game... please wait."
			// We need a local copy of state/topo for the goroutine to be safe
			s := m.state.DeepCopy()
			t := m.topology // Topology is mostly read-only except for Port override which we don't change here
			return m, func() tea.Msg {
				return simulationMsg(s.Simulate(&t))
			}
		case "up", "down":
			if m.viewMode == 1 {
				if m.tradeStep < 2 {
					if msg.String() == "up" {
						m.tradeCursor = (m.tradeCursor - 1 + 5) % 5
					} else {
						m.tradeCursor = (m.tradeCursor + 1) % 5
					}
				} else if len(m.state.Meta.PendingOffers) > 0 {
					if msg.String() == "up" {
						m.offerIdx = (m.offerIdx - 1 + len(m.state.Meta.PendingOffers)) % len(m.state.Meta.PendingOffers)
					} else {
						m.offerIdx = (m.offerIdx + 1) % len(m.state.Meta.PendingOffers)
					}
				}
			} else {
				m.moveCursor(msg.String())
			}
		case "left", "right":
			if m.viewMode == 0 {
				m.moveCursor(msg.String())
			}
		case "enter":
			if m.viewMode == 1 {
				res := []string{"wood", "brick", "sheep", "wheat", "ore"}
				if m.tradeStep == 0 {
					m.tradeGive = res[m.tradeCursor]
					m.tradeStep = 1
				} else if m.tradeStep == 1 {
					m.tradeGet = res[m.tradeCursor]
					m.tradeStep = 2
				}
			} else {
				m.handleAction()
			}
		case "v":
			if m.state.Meta.Status == "finished" {
				m.viewMode = 1 // Set to 1 to show board view
				m.message = "Viewing final board."
			}
		case "1":
			m.viewMode = 0
		case "2":
			m.viewMode = 1
		case "r":
			m.runDM("roll")
		case "e":
			m.runDM("end_turn")
		case "g":
			if m.state.Meta.Status == "invitation" {
				m.runDM("join", fmt.Sprintf("guest-%d", len(m.state.Players)+1), "guest")
			}
		case "b":
			if m.state.Meta.Status == "invitation" {
				m.runDM("join", fmt.Sprintf("bot-%d", len(m.state.Players)+1), "bot")
			} else if m.state.Meta.Phase == "action" {
				m.runDM("move", m.state.Meta.CurrentPlayerID, "buy_dev_card", "")
			}
		case "k":
			if m.state.Meta.Phase == "action" {
				m.runDM("move", m.state.Meta.CurrentPlayerID, "play_dev_card", "knight")
			}
		case "t":
			if m.viewMode == 1 {
				m.tradeStep = 0
				m.tradeGive = ""
				m.tradeGet = ""
			}
		case "u":
			if m.state.Meta.Status == "invitation" {
				if os.Getenv("GIT_TOKEN") == "" {
					m.message = "Error: GIT_TOKEN env var not set"
					return m, nil
				}
				m.isPrompting = true
				m.inputText = ""
			}
		case "x":
			if m.state.Meta.Status == "invitation" {
				m.runDM("remove_player")
			}
		case "s":
			m.runDM("begin")
		case "c":
			// Cheat resources for testing
			m.runDM("move", m.state.Meta.CurrentPlayerID, "cheat_resources")
		case "3", "4", "5":
			// No-op or future pages
		case "B", "P", "S", "A", "R": // Trading actions
			if m.viewMode == 1 {
				activePID := m.state.Meta.CurrentPlayerID
				switch msg.String() {
				case "B":
					if m.tradeStep == 2 {
						m.runDM("move", activePID, "trade_bank", m.tradeGive+":"+m.tradeGet)
						m.tradeStep = 0
					}
				case "P":
					if m.tradeStep == 2 {
						m.runDM("move", activePID, "trade_port", m.tradeGive+":"+m.tradeGet)
						m.tradeStep = 0
					}
				case "S":
					// Submit Offer (Non-active players)
					var fromPID string
					for _, p := range m.state.Players {
						if p.ID != activePID {
							fromPID = p.ID
							break
						}
					}
					if fromPID != "" && m.tradeStep == 2 {
						m.runDM("move", fromPID, "submit_trade_offer", fmt.Sprintf("%s:%s:1:1", m.tradeGive, m.tradeGet))
						m.tradeStep = 0
					}
				case "A":
					// Accept Offer (Active player)
					if len(m.state.Meta.PendingOffers) > 0 {
						offer := m.state.Meta.PendingOffers[m.offerIdx]
						m.runDM("move", activePID, "accept_trade_offer", offer.ID)
						m.offerIdx = 0
					}
				case "R":
					// Reject Offer (Active player)
					if len(m.state.Meta.PendingOffers) > 0 {
						offer := m.state.Meta.PendingOffers[m.offerIdx]
						m.runDM("move", activePID, "reject_trade_offer", offer.ID)
						m.offerIdx = 0
					}
				}
			}
		}
	}
	return m, nil
}

func (m *model) runDM(args ...string) {
	if len(args) < 1 {
		return
	}
	action := args[0]
	var err error

	switch action {
	case "init":
		m.state.Init(&m.topology)
	case "join":
		pType := "guest"
		if len(args) > 2 {
			pType = args[2]
		}
		err = m.state.Join(args[1], pType)
	case "remove_player":
		err = m.state.RemovePlayer()
	case "simulate":
		m.state.Simulate(&m.topology)
	case "begin":
		err = m.state.Begin()
	case "roll":
		m.state.Roll(0, &m.topology)
	case "end_turn":
		m.state.EndTurn()
	case "move":
		err = m.state.Move(args[1], args[2], args[3], &m.topology)
	}

	if err != nil {
		m.message = fmt.Sprintf("Error: %v", err)
	} else {
		m.message = fmt.Sprintf("DM Action %s completed.", action)
		m.relinkPorts() // Ensure visuals stay sync'd
		
		// Rebuild player styles every time to catch new players
		m.playerStyles = make(map[string]lipgloss.Style)
		for i, p := range m.state.Players {
			m.playerStyles[p.ID] = playerColorStyles[i%len(playerColorStyles)]
		}

		m.saveState()
	}
}

func (m *model) saveState() {
	savePath := "game.yaml"
	out, _ := yaml.Marshal(m.state)
	os.WriteFile(savePath, out, 0644)
}

func (m *model) handleAction() {
	action := ""
	playerID := m.state.Meta.CurrentPlayerID

	if m.selectedType == "vertex" {
		// Contextual: if settlement exists and is ours, build city. Otherwise settlement.
		if v, ok := m.state.Board.Vertices[m.selectedID]; ok && v.OwnerID == playerID && v.Type == "settlement" {
			action = "build_city"
		} else {
			action = "build_settlement"
		}
	} else if m.selectedType == "edge" {
		action = "build_road"
	}

	if action == "" {
		return
	}

	// 1. Local Validation (Simplified for now)
	if err := m.validateMove(action); err != nil {
		m.message = fmt.Sprintf("Rejected: %v", err)
		return
	}

	// 2. Authoritative Move
	m.runDM("move", playerID, action, m.selectedID)
}

func (m *model) refreshState() {
	gameData, err := os.ReadFile("game.yaml")
	if err != nil {
		m.message = fmt.Sprintf("Error refreshing state: %v", err)
		return
	}

	var state GameState
	if err := yaml.Unmarshal(gameData, &state); err != nil {
		m.message = fmt.Sprintf("Error unmarshaling state: %v", err)
		return
	}
	m.state = state
	m.relinkPorts()

	// Refresh player styles in case they changed (unlikely but safe)
	m.playerStyles = make(map[string]lipgloss.Style)
	for i, p := range m.state.Players {
		m.playerStyles[p.ID] = playerColorStyles[i%len(playerColorStyles)]
	}
}

func (m *model) validateMove(action string) error {
	return m.state.validateMoveLocal(m.state.Meta.CurrentPlayerID, action, m.selectedID, &m.topology)
}

func (m *model) moveCursor(dir string) {
	var curX, curY int
	if m.selectedType == "vertex" {
		v := m.topology.Vertices[m.selectedID]
		curX, curY = v.X, v.Y
	} else {
		e := m.topology.Edges[m.selectedID]
		curX, curY = e.X, e.Y
	}

	bestID := ""
	bestType := ""
	minDist := 1000.0

	if m.selectedType == "vertex" {
		// Only look at adjacent edges
		vTopo := m.topology.Vertices[m.selectedID]
		for _, eID := range vTopo.AdjacentEdges {
			e := m.topology.Edges[eID]
			dist := m.calculateDistance(curX, curY, e.X, e.Y, dir)
			if dist < minDist {
				minDist = dist
				bestID = eID
				bestType = "edge"
			}
		}
	} else {
		// Only look at adjacent vertices
		eTopo := m.topology.Edges[m.selectedID]
		for _, vID := range eTopo.AdjacentVertices {
			v := m.topology.Vertices[vID]
			dist := m.calculateDistance(curX, curY, v.X, v.Y, dir)
			if dist < minDist {
				minDist = dist
				bestID = vID
				bestType = "vertex"
			}
		}
	}

	if bestID != "" {
		m.selectedID = bestID
		m.selectedType = bestType
	}
}

func (m model) calculateDistance(curX, curY, targetX, targetY int, dir string) float64 {
	dx := float64(targetX - curX)
	dy := float64(targetY - curY)

	switch dir {
	case "up":
		if dy >= 0 {
			return 1000
		}
		return -dy*2 + (dx * dx / 10.0) // Prioritize vertical alignment
	case "down":
		if dy <= 0 {
			return 1000
		}
		return dy*2 + (dx * dx / 10.0)
	case "left":
		if dx >= 0 {
			return 1000
		}
		return -dx + (dy * dy * 10.0) // Prioritize horizontal alignment
	case "right":
		if dx <= 0 {
			return 1000
		}
		return dx + (dy * dy * 10.0)
	}
	return 1000
}

var ansiRegex = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func getRuneWidth(r rune) int {
	if w, ok := activeTheme.Widths[string(r)]; ok {
		return w
	}
	w := runewidth.RuneWidth(r)
	if w == 0 {
		return 1 // Fallback for zero-width chars that we want to show
	}
	return w
}

func stringVisualWidth(s string) int {
	plain := stripAnsi(s)
	// Check for full-string override (useful for specific icons)
	if w, ok := activeTheme.Widths[plain]; ok {
		return w
	}
	
	w := 0
	runes := []rune(plain)
	for i := 0; i < len(runes); i++ {
		// Greedy match multicharacter overrides
		found := false
		for l := 4; l >= 2; l-- {
			if i+l <= len(runes) {
				substr := string(runes[i : i+l])
				if overrideW, ok := activeTheme.Widths[substr]; ok {
					w += overrideW
					i += (l - 1)
					found = true
					break
				}
			}
		}
		if !found {
			w += getRuneWidth(runes[i])
		}
	}
	return w
}

var xterm256 = []color.RGBA{
	{0, 0, 0, 255}, {128, 0, 0, 255}, {0, 128, 0, 255}, {128, 128, 0, 255}, {0, 0, 128, 255}, {128, 0, 128, 255}, {0, 128, 128, 255}, {192, 192, 192, 255},
	{128, 128, 128, 255}, {255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 0, 255}, {0, 0, 255, 255}, {255, 0, 255, 255}, {0, 255, 255, 255}, {255, 255, 255, 255},
	{0, 0, 0, 255}, {0, 0, 95, 255}, {0, 0, 135, 255}, {0, 0, 175, 255}, {0, 0, 215, 255}, {0, 0, 255, 255}, {0, 95, 0, 255}, {0, 95, 95, 255},
	{0, 95, 135, 255}, {0, 95, 175, 255}, {0, 95, 215, 255}, {0, 95, 255, 255}, {0, 135, 0, 255}, {0, 135, 95, 255}, {0, 135, 135, 255}, {0, 135, 175, 255},
	{0, 135, 215, 255}, {0, 135, 255, 255}, {0, 175, 0, 255}, {0, 175, 95, 255}, {0, 175, 135, 255}, {0, 175, 175, 255}, {0, 175, 215, 255}, {0, 175, 255, 255},
	{0, 215, 0, 255}, {0, 215, 95, 255}, {0, 215, 135, 255}, {0, 215, 175, 255}, {0, 215, 215, 255}, {0, 215, 255, 255}, {0, 255, 0, 255}, {0, 255, 95, 255},
	{0, 255, 135, 255}, {0, 255, 175, 255}, {0, 255, 215, 255}, {0, 255, 255, 255}, {95, 0, 0, 255}, {95, 0, 95, 255}, {95, 0, 135, 255}, {95, 0, 175, 255},
	{95, 0, 215, 255}, {95, 0, 255, 255}, {95, 95, 0, 255}, {95, 95, 95, 255}, {95, 95, 135, 255}, {95, 95, 175, 255}, {95, 95, 215, 255}, {95, 95, 255, 255},
	{95, 135, 0, 255}, {95, 135, 95, 255}, {95, 135, 135, 255}, {95, 135, 175, 255}, {95, 135, 215, 255}, {95, 135, 255, 255}, {95, 175, 0, 255}, {95, 175, 95, 255},
	{95, 175, 135, 255}, {95, 175, 175, 255}, {95, 175, 215, 255}, {95, 175, 255, 255}, {95, 215, 0, 255}, {95, 215, 95, 255}, {95, 215, 135, 255}, {95, 215, 175, 255},
	{95, 215, 215, 255}, {95, 215, 255, 255}, {95, 255, 0, 255}, {95, 255, 95, 255}, {95, 255, 135, 255}, {95, 255, 175, 255}, {95, 255, 215, 255}, {95, 255, 255, 255},
	{135, 0, 0, 255}, {135, 0, 95, 255}, {135, 0, 135, 255}, {135, 0, 175, 255}, {135, 0, 215, 255}, {135, 0, 255, 255}, {135, 95, 0, 255}, {135, 95, 95, 255},
	{135, 95, 135, 255}, {135, 95, 175, 255}, {135, 95, 215, 255}, {135, 95, 255, 255}, {135, 135, 0, 255}, {135, 135, 95, 255}, {135, 135, 135, 255}, {135, 135, 175, 255},
	{135, 135, 215, 255}, {135, 135, 255, 255}, {135, 175, 0, 255}, {135, 175, 95, 255}, {135, 175, 135, 255}, {135, 175, 175, 255}, {135, 175, 215, 255}, {135, 175, 255, 255},
	{135, 215, 0, 255}, {135, 215, 95, 255}, {135, 215, 135, 255}, {135, 215, 175, 255}, {135, 215, 215, 255}, {135, 215, 255, 255}, {135, 255, 0, 255}, {135, 255, 95, 255},
	{135, 255, 135, 255}, {135, 255, 175, 255}, {135, 255, 215, 255}, {135, 255, 255, 255}, {175, 0, 0, 255}, {175, 0, 95, 255}, {175, 0, 135, 255}, {175, 0, 175, 255},
	{175, 0, 215, 255}, {175, 0, 255, 255}, {175, 95, 0, 255}, {175, 95, 95, 255}, {175, 95, 135, 255}, {175, 95, 175, 255}, {175, 95, 215, 255}, {175, 95, 255, 255},
	{175, 135, 0, 255}, {175, 135, 95, 255}, {175, 135, 135, 255}, {175, 135, 175, 255}, {175, 135, 215, 255}, {175, 135, 255, 255}, {175, 175, 0, 255}, {175, 175, 95, 255},
	{175, 175, 135, 255}, {175, 175, 175, 255}, {175, 175, 215, 255}, {175, 175, 255, 255}, {175, 215, 0, 255}, {175, 215, 95, 255}, {175, 215, 135, 255}, {175, 215, 175, 255},
	{175, 215, 215, 255}, {175, 215, 255, 255}, {175, 255, 0, 255}, {175, 255, 95, 255}, {175, 255, 135, 255}, {175, 255, 175, 255}, {175, 255, 215, 255}, {175, 255, 255, 255},
	{215, 0, 0, 255}, {215, 0, 95, 255}, {215, 0, 135, 255}, {215, 0, 175, 255}, {215, 0, 215, 255}, {215, 0, 255, 255}, {215, 95, 0, 255}, {215, 95, 95, 255},
	{215, 95, 135, 255}, {215, 95, 175, 255}, {215, 95, 215, 255}, {215, 95, 255, 255}, {215, 135, 0, 255}, {215, 135, 95, 255}, {215, 135, 135, 255}, {215, 135, 175, 255},
	{215, 135, 215, 255}, {215, 135, 255, 255}, {215, 175, 0, 255}, {215, 175, 95, 255}, {215, 175, 135, 255}, {215, 175, 175, 255}, {215, 175, 215, 255}, {215, 175, 255, 255},
	{215, 215, 0, 255}, {215, 215, 95, 255}, {215, 215, 135, 255}, {215, 215, 175, 255}, {215, 215, 215, 255}, {215, 215, 255, 255}, {215, 255, 0, 255}, {215, 255, 95, 255},
	{215, 255, 135, 255}, {215, 255, 175, 255}, {215, 255, 215, 255}, {215, 255, 255, 255}, {255, 0, 0, 255}, {255, 0, 95, 255}, {255, 0, 135, 255}, {255, 0, 175, 255},
	{255, 0, 215, 255}, {255, 0, 255, 255}, {255, 95, 0, 255}, {255, 95, 95, 255}, {255, 95, 135, 255}, {255, 95, 175, 255}, {255, 95, 215, 255}, {255, 95, 255, 255},
	{255, 135, 0, 255}, {255, 135, 95, 255}, {255, 135, 135, 255}, {255, 135, 175, 255}, {255, 135, 215, 255}, {255, 135, 255, 255}, {255, 175, 0, 255}, {255, 175, 95, 255},
	{255, 175, 135, 255}, {255, 175, 175, 255}, {255, 175, 215, 255}, {255, 175, 255, 255}, {255, 215, 0, 255}, {255, 215, 95, 255}, {255, 215, 135, 255}, {255, 215, 175, 255},
	{255, 215, 215, 255}, {255, 215, 255, 255}, {255, 255, 0, 255}, {255, 255, 95, 255}, {255, 255, 135, 255}, {255, 255, 175, 255}, {255, 255, 215, 255}, {255, 255, 255, 255},
	{8, 8, 8, 255}, {18, 18, 18, 255}, {28, 28, 28, 255}, {38, 38, 38, 255}, {48, 48, 48, 255}, {58, 58, 58, 255}, {68, 68, 68, 255}, {78, 78, 78, 255},
	{88, 88, 88, 255}, {98, 98, 98, 255}, {108, 108, 108, 255}, {118, 118, 118, 255}, {128, 128, 128, 255}, {138, 138, 138, 255}, {148, 148, 148, 255}, {158, 148, 158, 255},
	{168, 168, 168, 255}, {178, 178, 178, 255}, {188, 188, 188, 255}, {198, 198, 198, 255}, {208, 208, 208, 255}, {218, 218, 218, 255}, {228, 218, 228, 255}, {238, 238, 238, 255},
}

func (m model) renderToBuffer() *GridBuffer {
	v := m.View()
	lines := strings.Split(v, "\n")
	buf := NewGridBuffer(m.width, len(lines))

	for y, line := range lines {
		if y >= buf.Height {
			break
		}
		
		fg := color.RGBA{255, 255, 255, 255}
		bg := color.RGBA{30, 30, 30, 255}
		
		runes := []rune(line)
		x := 0
		for i := 0; i < len(runes); i++ {
			if x >= buf.Width {
				break
			}
			r := runes[i]
			if r == '\x1b' {
				if i+2 < len(runes) && runes[i+1] == '[' {
					j := i + 2
					code := ""
					for j < len(runes) && runes[j] != 'm' {
						code += string(runes[j])
						j++
					}
					parts := strings.Split(code, ";")
					for k := 0; k < len(parts); k++ {
						p := parts[k]
						switch p {
						case "0":
							fg = color.RGBA{255, 255, 255, 255}
							bg = color.RGBA{30, 30, 30, 255}
						case "38": // FG extended
							if k+2 < len(parts) && parts[k+1] == "5" {
								idx, _ := strconv.Atoi(parts[k+2])
								if idx >= 0 && idx < 256 {
									fg = xterm256[idx]
								}
								k += 2
							} else if k+4 < len(parts) && parts[k+1] == "2" {
								r, _ := strconv.Atoi(parts[k+2])
								g, _ := strconv.Atoi(parts[k+3])
								b, _ := strconv.Atoi(parts[k+4])
								fg = color.RGBA{uint8(r), uint8(g), uint8(b), 255}
								k += 4
							}
						case "48": // BG extended
							if k+2 < len(parts) && parts[k+1] == "5" {
								idx, _ := strconv.Atoi(parts[k+2])
								if idx >= 0 && idx < 256 {
									bg = xterm256[idx]
								}
								k += 2
							} else if k+4 < len(parts) && parts[k+1] == "2" {
								r, _ := strconv.Atoi(parts[k+2])
								g, _ := strconv.Atoi(parts[k+3])
								b, _ := strconv.Atoi(parts[k+4])
								bg = color.RGBA{uint8(r), uint8(g), uint8(b), 255}
								k += 4
							}
						case "30", "31", "32", "33", "34", "35", "36", "37":
							idx, _ := strconv.Atoi(p)
							fg = xterm256[idx-30]
						case "90", "91", "92", "93", "94", "95", "96", "97":
							idx, _ := strconv.Atoi(p)
							fg = xterm256[idx-90+8]
						case "40", "41", "42", "43", "44", "45", "46", "47":
							idx, _ := strconv.Atoi(p)
							bg = xterm256[idx-40]
						case "100", "101", "102", "103", "104", "105", "106", "107":
							idx, _ := strconv.Atoi(p)
							bg = xterm256[idx-100+8]
						}
					}
					i = j
					continue
				}
			}
			
			w := getRuneWidth(r)
			buf.Cells[y][x] = Cell{
				Rune: r,
				FG:   fg,
				BG:   bg,
			}
			if w > 1 {
				for k := 1; k < w && x+k < buf.Width; k++ {
					buf.Cells[y][x+k] = Cell{Rune: 0, FG: fg, BG: bg}
				}
				x += w
			} else {
				x++
			}
		}
	}
	return buf
}

func (m model) renderBoard() string {
	lines := strings.Split(m.board, "\n")
	grid := make([][]string, len(lines))

	for i, line := range lines {
		runes := []rune(line)
		grid[i] = make([]string, len(runes))
		for j, r := range runes {
			grid[i][j] = string(r)
		}
	}

	applyStyle := func(x, y int, length int, style lipgloss.Style, newRunes string) {
		if y < 0 || y >= len(grid) {
			return
		}
		newRuneSlice := []rune(newRunes)
		currentRuneIdx := 0
		visualOffset := 0
		for visualOffset < length {
			charIdx := x + visualOffset
			if charIdx < 0 {
				visualOffset++
				continue
			}
			
			if charIdx >= len(grid[y]) {
				if charIdx >= 150 { break }
				oldLen := len(grid[y])
				for k := 0; k <= charIdx-oldLen; k++ {
					grid[y] = append(grid[y], " ")
				}
			}

			if currentRuneIdx < len(newRuneSlice) {
				r := newRuneSlice[currentRuneIdx]
				rendered := style.Render(string(r))
				w := getRuneWidth(r)
				
				grid[y][charIdx] = rendered
				if w > 1 {
					for j := 1; j < w && charIdx+j < len(grid[y]); j++ {
						grid[y][charIdx+j] = ""
					}
					visualOffset += w
				} else {
					visualOffset++
				}

				currentRuneIdx++
			} else {
				if grid[y][charIdx] != "" {
					grid[y][charIdx] = style.Render(grid[y][charIdx])
				}
				visualOffset++
			}
		}
	}

	// 1. Render Hexes (Background)
	for _, hState := range m.state.Board.Hexes {
		var avgX, avgY float64
		verts := strings.Split(hState.Vertices, ",")
		for _, vID := range verts {
			v := m.topology.Vertices[vID]
			avgX += float64(v.X)
			avgY += float64(v.Y)
		}
		cX := int(2.0 * (avgX / 6.0)) + 1
		cY := int(1.5 * (avgY / 6.0)) + 3 // Adjusting to the vertex-y line

		res, ok := resourceStyles[hState.Resource]
		if !ok {
			res = resourceStyles["desert"]
		}
		style := lipgloss.NewStyle().Foreground(res.Color)

		// Top lines
		applyStyle(cX-2, cY-2, 5, style, "▟███▙")
		applyStyle(cX-3, cY-1, 7, style, "▟█████▙")

		// Middle line
		var midStr string
		if hState.Robber {
			midStr = "▟██ ROB ▙"
		} else if hState.Token > 0 {
			midStr = fmt.Sprintf("▟█%02d█%s█▙", hState.Token, res.Icon)
		} else {
			midStr = fmt.Sprintf("▟███%s██▙", res.Icon)
		}
		applyStyle(cX-4, cY, 9, style, midStr)

		// Bottom lines
		applyStyle(cX-4, cY+1, 9, style, "▜███████▛")
		applyStyle(cX-3, cY+2, 7, style, "▜█████▛")
	}

	// 2. Render Ports (Replace P1-P9 markers from board.txt)
	for y := 0; y < len(grid); y++ {
		for x := 0; x < len(grid[y]); x++ {
			if grid[y][x] == "P" {
				portID := ""
				digitX, digitY := -1, -1
				// Check next char
				if x+1 < len(grid[y]) && grid[y][x+1] >= "1" && grid[y][x+1] <= "9" {
					portID = "P" + grid[y][x+1]
					digitX, digitY = x+1, y
				} else if y+1 < len(grid) && x < len(grid[y+1]) && grid[y+1][x] >= "1" && grid[y+1][x] <= "9" {
					// Check char below
					portID = "P" + grid[y+1][x]
					digitX, digitY = x, y+1
				}

				if portID != "" {
					if port, ok := m.topology.Ports[portID]; ok {
						icon := activeTheme.Board["port"]
						style := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
						
						// Use vertex port info (which is already overridden by game state)
						// Pick first vertex of this port to get the type
						if len(port.Vertices) > 0 {
							vID := port.Vertices[0]
							if v, ok := m.topology.Vertices[vID]; ok && v.Port != "" {
								if res, ok := resourceStyles[v.Port]; ok {
									icon = res.Icon
									style = style.Foreground(res.Color)
								}
							}
						}

						// Clear the 'P' and the digit
						grid[y][x] = " "
						if digitX != -1 {
							grid[digitY][digitX] = " "
						}
						// Apply icon
						applyStyle(x, y, stringVisualWidth(icon), style, icon)
					}
				}
			}
		}
	}

	// 3. Render Edges
	for id, eState := range m.state.Board.Edges {
		topo, ok := m.topology.Edges[id]
		if !ok {
			continue
		}
		style, owned := m.playerStyles[eState.OwnerID]
		if !owned {
			continue
		}

		charX := 2*topo.X + 1
		if topo.Y%2 == 0 { // Horizontal
			charY := int(1.5*float64(topo.Y)) + 3
			applyStyle(charX-1, charY, 3, style, "───")
		} else { // Diagonal
			yTop := int(1.5*float64(topo.Y-1)) + 4
			yBot := int(1.5*float64(topo.Y-1)) + 5
			v1 := m.topology.Vertices[topo.AdjacentVertices[0]]
			v2 := m.topology.Vertices[topo.AdjacentVertices[1]]
			
			slope := "╲"
			if (v1.X < v2.X && v1.Y > v2.Y) || (v2.X < v1.X && v2.Y > v1.Y) {
				slope = "╱"
			}
			
			if slope == "╱" {
				applyStyle(charX+1, yTop, 1, style, slope)
				applyStyle(charX, yBot, 1, style, slope)
				applyStyle(charX-1, yBot+1, 1, style, slope)
			} else {
				applyStyle(charX-1, yTop, 1, style, slope)
				applyStyle(charX, yBot, 1, style, slope)
				applyStyle(charX+1, yBot+1, 1, style, slope)
			}
		}
	}

	// 4. Render Vertices
	for id, vState := range m.state.Board.Vertices {
		topo, ok := m.topology.Vertices[id]
		if !ok {
			continue
		}
		style, owned := m.playerStyles[vState.OwnerID]
		if !owned {
			continue
		}

		charX := 2*topo.X + 1
		charY := int(1.5*float64(topo.Y)) + 3
		symbol := activeTheme.Board["settlement"]
		if vState.Type == "city" {
			symbol = activeTheme.Board["city"]
		}
		applyStyle(charX, charY, stringVisualWidth(symbol), style, symbol)
	}

	// 5. Render Cursor (OVERRIDE)
	effectiveCursorStyle := cursorStyle
	if m.state.Meta.Status == "setup" {
		for i, p := range m.state.Players {
			if p.ID == m.state.Meta.CurrentPlayerID {
				effectiveCursorStyle = playerColorStyles[i%len(playerColorStyles)].Copy().
					Background(lipgloss.Color("8")). // Dark grey background for cursor
					Bold(true)
				break
			}
		}
	}

	if m.selectedType == "vertex" {
		topo := m.topology.Vertices[m.selectedID]
		charX := 2*topo.X + 1
		charY := int(1.5*float64(topo.Y)) + 3

		symbol := activeTheme.Board["settlement"]
		if vState, ok := m.state.Board.Vertices[m.selectedID]; ok && vState.Type == "city" {
			symbol = activeTheme.Board["city"]
		}
		applyStyle(charX, charY, stringVisualWidth(symbol), effectiveCursorStyle, symbol)
	} else if m.selectedType == "edge" {
		topo := m.topology.Edges[m.selectedID]
		charX := 2*topo.X + 1
		if topo.Y%2 == 0 { // Horizontal
			charY := int(1.5*float64(topo.Y)) + 3
			applyStyle(charX-1, charY, 3, effectiveCursorStyle, "───")
		} else { // Diagonal
			yTop := int(1.5*float64(topo.Y-1)) + 4
			yBot := int(1.5*float64(topo.Y-1)) + 5
			v1 := m.topology.Vertices[topo.AdjacentVertices[0]]
			v2 := m.topology.Vertices[topo.AdjacentVertices[1]]
			slope := "╲"
			if (v1.X < v2.X && v1.Y > v2.Y) || (v2.X < v1.X && v2.Y > v1.Y) {
				slope = "╱"
			}
			if slope == "╱" {
				applyStyle(charX+1, yTop, 1, effectiveCursorStyle, slope)
				applyStyle(charX, yBot, 1, effectiveCursorStyle, slope)
			} else {
				applyStyle(charX-1, yTop, 1, effectiveCursorStyle, slope)
				applyStyle(charX, yBot, 1, effectiveCursorStyle, slope)
			}
		}
	}

	// Reconstruct the board
	var sb strings.Builder
	for i, row := range grid {
		lineStr := ""
		visualWidth := 0
		for _, cell := range row {
			if cell == "" {
				continue
			}
			lineStr += cell
			visualWidth += stringVisualWidth(cell)
		}
		sb.WriteString(lineStr)
		// Pad every line to exactly 60 visual cells
		if visualWidth < 60 {
			sb.WriteString(strings.Repeat(" ", 60-visualWidth))
		}
		if i < len(grid)-1 {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func (m model) renderTradeView() string {
	var sb strings.Builder
	pID := m.state.Meta.CurrentPlayerID
	var player Player
	found := false
	for _, p := range m.state.Players {
		if p.ID == pID {
			player = p
			found = true
			break
		}
	}

	if !found {
		return "Player not found"
	}

	sb.WriteString(titleStyle.Render("TRADING CENTER"))
	sb.WriteString("\n\n")

	// Current Hand
	sb.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("YOUR RESOURCES") + "\n")
	resOrder := []string{"wood", "brick", "sheep", "wheat", "ore"}
	for i, resName := range resOrder {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(resourceStyles[resName].Color)
		if i == m.tradeCursor {
			cursor = "> "
			style = style.Copy().Bold(true).Background(lipgloss.Color("8"))
		}
		res := resourceStyles[resName]
		count := player.Resources[resName]
		sb.WriteString(style.Render(fmt.Sprintf("%s %s %-7s : %d", cursor, res.Icon, strings.Title(resName), count)) + "\n")
	}
	sb.WriteString("\n")

	// Trade Progress
	sb.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("CURRENT TRADE") + "\n")
	giveStr := "???"
	if m.tradeGive != "" {
		giveStr = strings.Title(m.tradeGive)
	}
	getStr := "???"
	if m.tradeGet != "" {
		getStr = strings.Title(m.tradeGet)
	}
	sb.WriteString(fmt.Sprintf(" Give: %s  ->  Receive: %s\n\n", giveStr, getStr))

	switch m.tradeStep {
	case 0:
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("STEP 1: Select resource to GIVE (Arrows + Enter)") + "\n")
	case 1:
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("STEP 2: Select resource to RECEIVE (Arrows + Enter)") + "\n")
	case 2:
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("STEP 3: Select trade partner") + "\n")
		
		if pID == m.state.Meta.CurrentPlayerID {
			sb.WriteString(" [B] Bank (4:1)\n")
			ports := m.state.GetPlayerPorts(pID, &m.topology)
			hasPort := false
			for _, p := range ports {
				if p == m.tradeGive || p == "3:1" {
					hasPort = true
					break
				}
			}
			if hasPort {
				sb.WriteString(" [P] Port (2:1 or 3:1)\n")
			}
		} else {
			sb.WriteString(" [S] Submit Offer to Active Player (1:1)\n")
		}
	}

	// Pending Offers Section
	if len(m.state.Meta.PendingOffers) > 0 {
		sb.WriteString("\n" + lipgloss.NewStyle().Bold(true).Underline(true).Render("PENDING OFFERS") + "\n")
		for i, offer := range m.state.Meta.PendingOffers {
			cursor := "  "
			style := lipgloss.NewStyle()
			if i == m.offerIdx {
				cursor = "> "
				style = style.Bold(true).Background(lipgloss.Color("8"))
			}
			line := fmt.Sprintf("%s %s offers %d %s for %d %s", 
				cursor, offer.FromPlayerID, offer.GiveAmount, offer.GiveResource, offer.GetAmount, offer.GetResource)
			sb.WriteString(style.Render(line) + "\n")
		}
		if pID == m.state.Meta.CurrentPlayerID {
			sb.WriteString("\n [A] Accept Selected  [R] Reject Selected\n")
		}
	} else {
		sb.WriteString("\nNo pending offers.\n")
	}

	sb.WriteString("\n [T] Reset Trade\n")
	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Press '1' to return to Board View."))

	return borderStyle.Width(m.width - 4).Height(m.height - 4).Render(sb.String())
}

func (m model) renderGameOver() string {
	var sb strings.Builder

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("9")).
		Padding(1, 4).
		MarginBottom(1).
		Render(" G A M E   O V E R ")

	sb.WriteString(title + "\n\n")

	// Find winner
	winnerID := "No one"
	maxVP := -1
	for _, p := range m.state.Players {
		if p.VP > maxVP {
			maxVP = p.VP
			winnerID = p.ID
		}
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render(fmt.Sprintf("WINNER: %s", winnerID)) + "\n\n")
	sb.WriteString(lipgloss.NewStyle().Underline(true).Render("FINAL STANDINGS:") + "\n")

	// Sort players by VP
	players := make([]Player, len(m.state.Players))
	copy(players, m.state.Players)
	sort.Slice(players, func(i, j int) bool {
		return players[i].VP > players[j].VP
	})

	for i, p := range players {
		style := playerColorStyles[i%len(playerColorStyles)]
		sb.WriteString(style.Render(fmt.Sprintf(" #%d: %-15s %2d VP (%s)", i+1, p.ID, p.VP, strings.ToUpper(p.Type))) + "\n")
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("Press 'v' to View Final Board, 'P' for Playback, 'I' for New Game, or 'Q' to Quit.") + "\n")

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(sb.String())
}

func (m model) View() string {
	if m.state.Meta.Status == "finished" && m.viewMode == 0 {
		return m.renderGameOver()
	}

	if m.viewMode == 1 && m.state.Meta.Status != "finished" {
		return m.renderTradeView()
	}

	// Board box
	boardView := lipgloss.NewStyle().
		Render(m.renderBoard()) // Removed MarginTop(1)

	// Dashboard box
	dashboardWidth := m.width - 74 - 2
	if dashboardWidth < 20 {
		dashboardWidth = 20
	}

	sectionStyle := lipgloss.NewStyle().Width(dashboardWidth)

	// --- All section definitions remain the same, only their join order changes ---

	// 1. Header Section
	var headerSB strings.Builder
	headerSB.WriteString(titleStyle.Render("GAME DASHBOARD") + "\n\n")
	phaseStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	if len(m.history) > 0 {
		status := "PAUSED"
		if m.isPlaying {
			status = "PLAYING"
		}
		headerSB.WriteString(phaseStyle.Copy().Foreground(lipgloss.Color("10")).Render("PLAYBACK ("+status+"):") + fmt.Sprintf(" Step %d/%d\n", m.historyIdx+1, len(m.history)))
	} else {
		headerSB.WriteString(phaseStyle.Render("PHASE:") + " " + strings.ToUpper(m.state.Meta.Phase) + "\n")
		headerSB.WriteString(phaseStyle.Render("STATE:") + " " + strings.ToUpper(m.state.Meta.Status) + "\n")
	}
	headerView := sectionStyle.Copy().Height(5).Render(headerSB.String())

	// 2. Controls Section
	var controlsSB strings.Builder
	controlsSB.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("CONTROLS") + "\n")
	controlsSB.WriteString(" Arrows : Navigate\n")
	controlsSB.WriteString(" Enter  : Build/Upgrade\n")
	if m.state.Meta.Status == "invitation" {
		controlsSB.WriteString(" G, B, U: Add Players\n")
		controlsSB.WriteString(" S      : Start Game\n")
		controlsSB.WriteString(" P      : Simulation\n")
	} else if m.state.Meta.Status == "finished" {
		controlsSB.WriteString(" 1      : Show Standings\n")
		controlsSB.WriteString(" v      : Show Board\n")
		controlsSB.WriteString(" I      : New Game\n")
	} else if m.state.Meta.Phase == "roll" {
		controlsSB.WriteString(" R      : Roll Dice\n")
	} else {
		controlsSB.WriteString(" 2      : Trading View\n")
		controlsSB.WriteString(" B      : Buy Dev Card\n")
		controlsSB.WriteString(" K      : Play Knight\n")
		controlsSB.WriteString(" E      : End Turn\n")
	}
	controlsSB.WriteString(" Q      : Quit\n")
	controlsView := sectionStyle.Copy().Height(10).Render(controlsSB.String())

	// 3. Selection Info Section (defined here, joined at bottom)
	var selectionSB strings.Builder
	selectionSB.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Render("SELECTED:") + " ")
	var coords string
	if m.selectedType == "vertex" {
		if len(m.selectedID) >= 4 {
			row := m.selectedID[1]
			colStr := strings.TrimLeft(m.selectedID[2:], "0")
			coords = fmt.Sprintf("%c%s", row, colStr)
		} else {
			coords = m.selectedID
		}
		if v, ok := m.topology.Vertices[m.selectedID]; ok && v.Port != "" {
			coords += fmt.Sprintf(" (Port: %s)", v.Port)
		}
	} else {
		topo := m.topology.Edges[m.selectedID]
		var parts []string
		for _, vID := range topo.AdjacentVertices {
			if len(vID) >= 4 {
				row := vID[1]
				colStr := strings.TrimLeft(vID[2:], "0")
				parts = append(parts, fmt.Sprintf("%c%s", row, colStr))
			}
		}
		coords = strings.Join(parts, "-")
	}
	selectionSB.WriteString(fmt.Sprintf("%s [%s]\n", m.selectedID, coords))
	selectionView := sectionStyle.Copy().Height(3).Render(selectionSB.String())

	// 4. Resource Legend Section (VERTICAL)
	var resourceSB strings.Builder
	resourceSB.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("RESOURCES & BANK") + "\n")
	resOrder := []string{"wood", "brick", "sheep", "wheat", "ore"}
	for _, resName := range resOrder {
		res := resourceStyles[resName]
		style := lipgloss.NewStyle().Foreground(res.Color)
		bank := MaxBank - m.state.GetTotalResources(resName)
		name := strings.Title(resName)
		if resName == "ore" {
			name = " " + name + " "
		}
		resourceSB.WriteString(style.Render(fmt.Sprintf("%s %-7s (%d)", res.Icon, name, bank)) + "\n")
	}
	resourceView := sectionStyle.Copy().Height(7).Render(resourceSB.String())

	// 5. Special VP Section
	var specialVPSB strings.Builder
	specialVPSB.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("SPECIAL VP") + "\n")
	armyStr := "Largest Army: "
	if m.state.Meta.LargestArmyPlayer != "" {
		armyStr += fmt.Sprintf("%s (%d)", m.state.Meta.LargestArmyPlayer, m.state.Meta.LargestArmyCount)
	} else {
		armyStr += "None"
	}
	specialVPSB.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render(armyStr) + "\n")
	roadStr := "Longest Road: "
	if m.state.Meta.LongestRoadPlayer != "" {
		roadStr += fmt.Sprintf("%s (%d)", m.state.Meta.LongestRoadPlayer, m.state.Meta.LongestRoadCount)
	} else {
		roadStr += "None"
	}
	specialVPSB.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Render(roadStr) + "\n")
	specialVPView := sectionStyle.Copy().Height(4).Render(specialVPSB.String())

	// Roll Section
	var rollSB strings.Builder
	rollSB.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("CURRENT ROLL") + "\n")
	if m.state.Meta.LastRoll1 > 0 {
		d1 := m.state.Meta.LastRoll1
		d2 := m.state.Meta.LastRoll2
		total := d1 + d2
		
		dice1 := toDice(d1)
		dice2 := toDice(d2)
		
		rollView := lipgloss.JoinHorizontal(lipgloss.Center,
			lipgloss.NewStyle().Padding(0, 1).Render(dice1),
			lipgloss.NewStyle().Bold(true).Render(" + "),
			lipgloss.NewStyle().Padding(0, 1).Render(dice2),
			lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf(" = %d", total)),
		)
		
		if total == 7 {
			rollView = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(rollView)
		}
		rollSB.WriteString(rollView + "\n")

		// Resources gained
		gains := m.state.GetRollResources(total)
		if len(gains) > 0 {
			playerGains := make(map[string][]string)
			for res, players := range gains {
				for pID, count := range players {
					style := resourceStyles[res]
					playerGains[pID] = append(playerGains[pID], fmt.Sprintf("%d%s", count, style.Icon))
				}
			}
			for pID, resList := range playerGains {
				rollSB.WriteString(fmt.Sprintf("%s: %s\n", pID, strings.Join(resList, ", ")))
			}
		} else if total != 7 {
			rollSB.WriteString("No resources produced.\n")
		} else {
			rollSB.WriteString("ROBBER ACTIVE!\n")
		}
	} else {
		rollSB.WriteString("No roll yet.\n")
	}
	lastRollView := sectionStyle.Copy().Height(6).Render(rollSB.String())

	// 6. Players Section
	var playersSB strings.Builder
	playersSB.WriteString(lipgloss.NewStyle().Bold(true).Underline(true).Render("PLAYERS") + "\n")
	for i, p := range m.state.Players {
		prefix := "  "
		isCurrent := p.ID == m.state.Meta.CurrentPlayerID
		style := playerColorStyles[i%len(playerColorStyles)]
		if isCurrent {
			prefix = activeTheme.UI["player_cursor"]
			style = style.Copy().Bold(true).Underline(true)
		}
		line1 := fmt.Sprintf("%s [%s] VP:%d", prefix, p.ID, p.VP)
		if m.state.Meta.LargestArmyPlayer == p.ID {
			line1 += " " + activeTheme.UI["largest_army"]
		}
		if m.state.Meta.LongestRoadPlayer == p.ID {
			line1 += " " + activeTheme.UI["longest_road"]
		}
		playersSB.WriteString(style.Render(line1) + "\n")
		if isCurrent {
			if p.Type == "git" {
				total := 0
				for _, count := range p.Resources {
					total += count
				}
				playersSB.WriteString(fmt.Sprintf("   Resources: %d cards (Private)\n", total))
			} else {
				resLine := fmt.Sprintf("   %s:%d %s:%d %s:%d %s:%d %s:%d",
					activeTheme.Resources["wood"], p.Resources["wood"],
					activeTheme.Resources["brick"], p.Resources["brick"],
					activeTheme.Resources["sheep"], p.Resources["sheep"],
					activeTheme.Resources["wheat"], p.Resources["wheat"],
					activeTheme.Resources["ore"], p.Resources["ore"])
				playersSB.WriteString(resLine + "\n")
			}

			// Add Dev Cards (Hidden for git users)
			var cardList []string
			totalCards := 0
			for card, count := range p.DevCards {
				if count > 0 {
					name := strings.Title(strings.ReplaceAll(card, "_", " "))
					cardList = append(cardList, fmt.Sprintf("%s:%d", name, count))
					totalCards += count
				}
			}
			for card, count := range p.NewDevCards {
				if count > 0 {
					name := strings.Title(strings.ReplaceAll(card, "_", " "))
					cardList = append(cardList, fmt.Sprintf("%s:%d(new)", name, count))
					totalCards += count
				}
			}
			if totalCards > 0 {
				if p.Type == "git" {
					playersSB.WriteString(fmt.Sprintf("   Cards: %d (Private)\n", totalCards))
				} else {
					playersSB.WriteString("   Cards: " + strings.Join(cardList, ", ") + "\n")
				}
			}
		}
	}
	playersView := sectionStyle.Copy().Height(10).Render(playersSB.String())

	// Combine Dashboard
	dashboardContent := lipgloss.JoinVertical(lipgloss.Left,
		headerView,
		controlsView,
		resourceView,
		specialVPView,
		lastRollView,
		playersView,
		selectionView, // MOVED TO BOTTOM
	)

	dashboardView := dashboardStyle.Copy().
		Width(dashboardWidth).
		Render(dashboardContent)

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, boardView, dashboardView)

	// 7. Status Section (FOOTER)
	var statusSB strings.Builder
	if len(m.history) > 0 {
		status := activeTheme.UI["paused"] + " PAUSED"
		if m.isPlaying {
			status = activeTheme.UI["playing"] + " PLAYING"
		}
		playbackInfo := fmt.Sprintf("Step %d/%d", m.historyIdx+1, len(m.history))
		controls := "[Space] Toggle Play/Pause | [←/→] Step"
		statusSB.WriteString(lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).Render(status),
			"  ",
			playbackInfo,
			"  |  ",
			lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(controls),
		))
	} else if m.message != "" {
		statusStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
		statusSB.WriteString(statusStyle.Render("STATUS: ") + m.message)
	}
	footerView := borderStyle.Copy().
		Width(m.width - 2).
		Render(statusSB.String())

	view := lipgloss.JoinVertical(lipgloss.Left, mainView, footerView)

	if m.isPrompting {
		promptStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("13")).
			Padding(1, 2).
			Background(lipgloss.Color("0"))

		promptBox := promptStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				lipgloss.NewStyle().Bold(true).Render("ENTER GIT USERNAME:"),
				"",
				lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("8")).Padding(0, 1).Render(m.inputText+"_"),
				"",
				lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("(Enter to Confirm, Esc to Cancel)"),
			),
		)

		// Center the prompt over the view
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, promptBox)
	}

	return view
}

func main() {
	loadTheme()
	if len(os.Args) > 1 && os.Args[1] == "dm" {
		handleDMCommand(os.Args[2:])
		return
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}

func handleDMCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: catan-go dm <action> [args...]")
		os.Exit(1)
	}

	action := args[0]
	
	// For CLI DM, we need to load state and topology
	var state GameState
	gameData, err := os.ReadFile("game.yaml")
	if err == nil {
		yaml.Unmarshal(gameData, &state)
	}

	var topo Topology
	topoData, err := os.ReadFile("topology.yaml")
	if err == nil {
		yaml.Unmarshal(topoData, &topo)
	}

	switch action {
	case "init":
		state.Init(&topo)
	case "join":
		if len(args) < 2 {
			fmt.Println("Usage: dm join <player_id> [type]")
			os.Exit(1)
		}
		pType := "guest"
		if len(args) > 2 {
			pType = args[2]
		}
		if err := state.Join(args[1], pType); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "remove_player":
		if err := state.RemovePlayer(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "begin":
		if err := state.Begin(); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "roll":
		forced := 0
		if len(args) > 1 {
			fmt.Sscanf(args[1], "%d", &forced)
		}
		state.Roll(forced, &topo)
	case "playback":
		handlePlayback(state, topo)
		return
	case "simulate":
		history := state.Simulate(&topo)
		os.MkdirAll("frames", 0755)
		// Clear frames
		files, _ := os.ReadDir("frames")
		for _, f := range files {
			os.Remove("frames/" + f.Name())
		}

		boardData, _ := os.ReadFile("board.txt")

		m := model{
			board:    string(boardData),
			topology: topo,
			width:    120,
			height:   60,
			history:  history,
		}
		m.relinkPorts()

		for i, s := range history {
			m.state = s
			m.historyIdx = i
			m.playerStyles = make(map[string]lipgloss.Style)
			for j, p := range s.Players {
				m.playerStyles[p.ID] = playerColorStyles[j%len(playerColorStyles)]
			}
			// Set a default selected vertex for board view
			for k := range m.topology.Vertices {
				m.selectedID = k
				m.selectedType = "vertex"
				break
			}

			buf := m.renderToBuffer()
			img, err := renderToImage(buf, 10, 20)
			if err != nil {
				fmt.Printf("Error rendering frame %d: %v\n", i, err)
				continue
			}
			filename := fmt.Sprintf("frames/frame_%04d.png", i)
			f, _ := os.Create(filename)
			png.Encode(f, img)
			f.Close()
		}
		fmt.Printf("Simulated game and rendered %d frames to frames/\n", len(history))
		return

	case "move":
		if len(args) < 4 {
			fmt.Println("Usage: dm move <player_id> <type> <target>")
			os.Exit(1)
		}
		if err := state.Move(args[1], args[2], args[3], &topo); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	case "end_turn":
		state.EndTurn()
	default:
		fmt.Printf("Unknown DM action: %s\n", action)
		os.Exit(1)
	}

	// Save back to game.yaml
	savePath := "game.yaml"
	out, _ := yaml.Marshal(state)
	os.WriteFile(savePath, out, 0644)
	fmt.Printf("DM Action %s completed.\n", action)
}

func handlePlayback(state GameState, topo Topology) {
	history := state.Replay(&topo)
	os.MkdirAll("frames", 0755)

	// We need board.txt for the model
	boardData, _ := os.ReadFile("board.txt")


	m := model{
		board:    string(boardData),
		topology: topo,
		width:    120,
		height:   60,
		history:  history,
	}
	m.relinkPorts()

	for i, s := range history {
		m.state = s
		m.historyIdx = i
		m.playerStyles = make(map[string]lipgloss.Style)
		for j, p := range s.Players {
			m.playerStyles[p.ID] = playerColorStyles[j%len(playerColorStyles)]
		}
		for k := range m.topology.Vertices {
			m.selectedID = k
			m.selectedType = "vertex"
			break
		}

		buf := m.renderToBuffer()
		img, err := renderToImage(buf, 10, 20)
		if err != nil {
			fmt.Printf("Error rendering frame %d: %v\n", i, err)
			continue
		}
		filename := fmt.Sprintf("frames/frame_%04d.png", i)
		f, _ := os.Create(filename)
		png.Encode(f, img)
		f.Close()
	}
	fmt.Printf("Rendered %d frames to frames/\n", len(history))
}

func toDice(n int) string {
	return fmt.Sprintf("%d", n)
}

func toASCII(r rune) string {
	s := string(r)
	
	switch s {
	case "①": return "1"
	case "②": return "2"
	case "③": return "3"
	case "④": return "4"
	case "⑤": return "5"
	case "⑥": return "6"
	case "⑦": return "7"
	case "⑧": return "8"
	case "⑨": return "9"
	case "⑩": return "10"
	case "⑪": return "11"
	case "⑫": return "12"
	case "Ⓐ": return "A"
	case "Ⓑ": return "B"
	case "Ⓒ": return "C"
	case "Ⓓ": return "D"
	case "Ⓔ": return "E"
	case "Ⓕ": return "F"
	case "Ⓖ": return "G"
	case "Ⓗ": return "H"
	case "Ⓘ": return "I"
	case "Ⓙ": return "J"
	case "Ⓚ": return "K"
	}

	return s
}

func renderToImage(buffer *GridBuffer, charWidth, charHeight int) (image.Image, error) {
	f, err := opentype.Parse(defaultFont)
	if err != nil {
		return nil, fmt.Errorf("could not parse embedded font: %v", err)
	}

	imgWidth := buffer.Width * charWidth
	imgHeight := buffer.Height * charHeight
	img := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))

	// Draw background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{30, 30, 30, 255}}, image.Point{}, draw.Src)

	// Create face
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(charHeight) * 0.8,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create font face: %v", err)
	}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.White,
		Face: face,
	}

	for y, row := range buffer.Cells {
		for x, cell := range row {
			if cell.Rune == 0 {
				continue
			}
			
			posX := x * charWidth
			posY := y * charHeight

			// Draw cell background if not default
			if cell.BG != (color.RGBA{30, 30, 30, 255}) && cell.BG != (color.RGBA{0, 0, 0, 0}) {
				draw.Draw(img, image.Rect(posX, posY, posX+charWidth, posY+charHeight), &image.Uniform{cell.BG}, image.Point{}, draw.Src)
			}

			// Draw character
			if cell.Rune != ' ' && cell.Rune != 0 {
				str := toASCII(cell.Rune)
				d.Src = &image.Uniform{cell.FG}
				
				// Standard center is charWidth/4 for 1 char.
				// For 2 chars (like "10"), we shift left half a char width more.
				dotX := fixed.I(posX) + (fixed.I(charWidth) - d.MeasureString(str))/2
				
				// Baseline adjustment
				d.Dot = fixed.Point26_6{
					X: dotX,
					Y: fixed.I(posY) + fixed.I(charHeight)*3/4,
				}
				d.DrawString(str)
			}
		}
	}

	return img, nil
}
