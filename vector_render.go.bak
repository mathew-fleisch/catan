package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"github.com/fogleman/gg"
	"gopkg.in/yaml.v3"
)

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
	Ports    map[string]string      `yaml:"ports"`
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
	Type          string         `yaml:"type"`
	Resources     map[string]int `yaml:"resources"`
	VP            int            `yaml:"vp"`
	SkipCount     int            `yaml:"skip_count"`
	DevCards      map[string]int `yaml:"dev_cards"`
	NewDevCards   map[string]int `yaml:"new_dev_cards"`
	KnightsPlayed int            `yaml:"knights_played"`
}

type LogEntry struct {
	Timestamp int64  `yaml:"timestamp"`
	PlayerID  string `yaml:"player_id"`
	Action    string `yaml:"action"`
	Data      string `yaml:"data"`
}

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
}

type PortTopology struct {
	Type     string   `yaml:"type"`
	Vertices []string `yaml:"vertices"`
}

var playerColors = []color.RGBA{
	{255, 50, 50, 255},   // Red
	{50, 255, 50, 255},   // Green
	{50, 100, 255, 255},  // Blue
	{255, 255, 50, 255},  // Yellow
}

type Theme struct {
	Resources map[string]string `yaml:"resources"`
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
	Resources: map[string]string{"wood": "W", "brick": "B", "sheep": "s", "wheat": "w", "ore": "O", "desert": "D"},
	Board:     map[string]string{"port": "S", "settlement": "o", "city": "H", "robber": "X"},
	UI:        map[string]string{"playing": ">>", "paused": "||", "largest_army": "[A]", "longest_road": "[R]", "player_cursor": ">>"},
}

func loadTheme() {
	data, err := os.ReadFile("themes.yaml")
	if err == nil {
		var config ThemeConfig
		if err := yaml.Unmarshal(data, &config); err == nil {
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
			}
		}
	}
}

func renderVectorFrame(state GameState, topo Topology, step, total int, filename string) {
	const W = 1400
	const H = 900
	const BoardWidth = 900
	const HexRadius = 55.0
	
	dc := gg.NewContext(W, H)
	dc.SetRGB(0.05, 0.05, 0.05)
	dc.Clear()

	fontPath := "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
	dc.LoadFontFace(fontPath, 20)

	offsetX := BoardWidth / 2.0
	offsetY := H / 2.0
	scaleX := 32.0
	scaleY := 32.0
	
	playerColorMap := make(map[string]color.Color)
	for i, p := range state.Players {
		playerColorMap[p.ID] = playerColors[i%len(playerColors)]
	}
	defaultColor := color.RGBA{200, 200, 200, 255}

	dc.SetRGB(0.1, 0.1, 0.1)
	dc.DrawRectangle(10, 10, BoardWidth-20, H-20)
	dc.Fill()

	for _, hState := range state.Board.Hexes {
		var avgX, avgY float64
		verts := strings.Split(hState.Vertices, ",")
		count := 0
		for _, vID := range verts {
			if v, ok := topo.Vertices[vID]; ok {
				avgX += float64(v.X)
				avgY += float64(v.Y)
				count++
			}
		}
		if count == 0 { continue }
		cX := (avgX / float64(count) - 12) * scaleX + offsetX
		cY := (avgY / float64(count) - 10) * scaleY + offsetY

		dc.DrawRegularPolygon(6, cX, cY, HexRadius, 0)
		switch hState.Resource {
		case "wood": dc.SetHexColor("#2e7d32")
		case "brick": dc.SetHexColor("#c62828")
		case "sheep": dc.SetHexColor("#9ccc65")
		case "wheat": dc.SetHexColor("#fbc02d")
		case "ore": dc.SetHexColor("#78909c")
		default: dc.SetHexColor("#555555")
		}
		dc.FillPreserve()
		dc.SetRGB(0, 0, 0)
		dc.SetLineWidth(1)
		dc.Stroke()

		if hState.Token > 0 {
			dc.SetRGB(1, 1, 1)
			dc.DrawCircle(cX, cY, 18)
			dc.Fill()
			dc.SetRGB(0, 0, 0)
			dc.DrawStringAnchored(fmt.Sprintf("%d", hState.Token), cX, cY-5, 0.5, 0.5)
			dc.SetRGB(0.2, 0.2, 0.2)
			dc.DrawStringAnchored(activeTheme.Resources[hState.Resource], cX, cY+12, 0.5, 0.5)
		}
		if hState.Robber {
			dc.SetRGB(0, 0, 0)
			dc.DrawCircle(cX, cY+20, 12)
			dc.Fill()
			dc.SetRGB(1, 1, 1)
			dc.DrawStringAnchored(activeTheme.Board["robber"], cX, cY+20, 0.5, 0.5)
		}
	}

	for id, eState := range state.Board.Edges {
		if eState.OwnerID == "" { continue }
		eTopo, ok := topo.Edges[id]
		if !ok || len(eTopo.AdjacentVertices) < 2 { continue }
		v1 := topo.Vertices[eTopo.AdjacentVertices[0]]
		v2 := topo.Vertices[eTopo.AdjacentVertices[1]]
		x1 := (float64(v1.X) - 12) * scaleX + offsetX
		y1 := (float64(v1.Y) - 10) * scaleY + offsetY
		x2 := (float64(v2.X) - 12) * scaleX + offsetX
		y2 := (float64(v2.Y) - 10) * scaleY + offsetY
		c, ok := playerColorMap[eState.OwnerID]
		if !ok { c = defaultColor }
		dc.SetColor(c)
		dc.SetLineWidth(10)
		dc.DrawLine(x1, y1, x2, y2)
		dc.Stroke()
		dc.SetRGB(0, 0, 0); dc.SetLineWidth(1); dc.DrawLine(x1, y1, x2, y2); dc.Stroke()
	}

	for id, vState := range state.Board.Vertices {
		if vState.OwnerID == "" { continue }
		vTopo, ok := topo.Vertices[id]
		if !ok { continue }
		vx := (float64(vTopo.X) - 12) * scaleX + offsetX
		vy := (float64(vTopo.Y) - 10) * scaleY + offsetY
		c, ok := playerColorMap[vState.OwnerID]
		if !ok { c = defaultColor }
		dc.SetColor(c)
		dc.DrawCircle(vx, vy, 15)
		dc.Fill()
		dc.SetRGB(0, 0, 0)
		icon := activeTheme.Board["settlement"]
		if vState.Type == "city" { icon = activeTheme.Board["city"] }
		dc.DrawStringAnchored(icon, vx, vy, 0.5, 0.5)
	}

	dashX := float64(BoardWidth + 20)
	dc.SetRGB(0.15, 0.15, 0.15); dc.DrawRectangle(dashX, 10, W-dashX-10, H-20); dc.Fill()
	dc.SetRGB(0.4, 0.4, 0.4); dc.DrawRectangle(dashX, 10, W-dashX-10, H-20); dc.Stroke()
	dc.SetRGB(1, 1, 1)
	dc.DrawStringAnchored("GAME DASHBOARD", dashX + (W-dashX)/2, 50, 0.5, 0.5)
	dc.DrawString(fmt.Sprintf("Step: %d / %d", step, total), dashX+20, 120)
	dc.DrawString(fmt.Sprintf("Phase: %s", strings.ToUpper(state.Meta.Phase)), dashX+20, 150)
	yPos := 210.0
	for i, p := range state.Players {
		dc.SetColor(playerColors[i%len(playerColors)]); dc.DrawCircle(dashX+30, yPos-5, 10); dc.Fill()
		dc.SetRGB(1, 1, 1)
		indicator := ""
		if p.ID == state.Meta.CurrentPlayerID { indicator = " " + activeTheme.UI["player_cursor"] }
		dc.DrawString(fmt.Sprintf("%s (VP: %d)%s", p.ID, p.VP, indicator), dashX+50, yPos)
		yPos += 25
		resStr := fmt.Sprintf("%s:%d %s:%d %s:%d %s:%d %s:%d", activeTheme.Resources["wood"], p.Resources["wood"], activeTheme.Resources["brick"], p.Resources["brick"], activeTheme.Resources["sheep"], p.Resources["sheep"], activeTheme.Resources["wheat"], p.Resources["wheat"], activeTheme.Resources["ore"], p.Resources["ore"])
		dc.SetRGB(0.8, 0.8, 0.8); dc.DrawString(resStr, dashX+50, yPos); yPos += 35
	}
	dc.SavePNG(filename)
}

func (s *GameState) DeepCopy() GameState {
	res := *s
	res.Players = make([]Player, len(s.Players))
	for i, p := range s.Players {
		res.Players[i] = p
		res.Players[i].Resources = make(map[string]int)
		for k, v := range p.Resources { res.Players[i].Resources[k] = v }
	}
	res.Board.Hexes = make(map[string]HexState)
	for k, v := range s.Board.Hexes { res.Board.Hexes[k] = v }
	res.Board.Vertices = make(map[string]VertexState)
	for k, v := range s.Board.Vertices { res.Board.Vertices[k] = v }
	res.Board.Edges = make(map[string]EdgeState)
	for k, v := range s.Board.Edges { res.Board.Edges[k] = v }
	return res
}

func (s *GameState) Replay(topo *Topology) []GameState {
	var history []GameState
	current := GameState{
		Board: BoardState{Hexes: make(map[string]HexState), Vertices: make(map[string]VertexState), Edges: make(map[string]EdgeState)},
		Meta: Meta{Status: "setup"},
	}
	for k, v := range s.Board.Hexes { current.Board.Hexes[k] = v }
	for k, v := range s.Board.Vertices { v.OwnerID = ""; v.Type = ""; current.Board.Vertices[k] = v }
	for k, v := range s.Board.Edges { v.OwnerID = ""; current.Board.Edges[k] = v }
	
	history = append(history, current.DeepCopy())
	for _, entry := range s.Log {
		switch entry.Action {
		case "join":
			current.Players = append(current.Players, Player{ID: entry.PlayerID, Resources: map[string]int{"wood":0,"brick":0,"sheep":0,"wheat":0,"ore":0}})
		case "begin":
			current.Meta.TurnOrder = strings.Split(entry.Data, ",")
			current.Meta.CurrentPlayerID = current.Meta.TurnOrder[0]
		case "build_settlement":
			v := current.Board.Vertices[entry.Data]; v.OwnerID = entry.PlayerID; v.Type = "settlement"; current.Board.Vertices[entry.Data] = v
		case "build_road":
			e := current.Board.Edges[entry.Data]; e.OwnerID = entry.PlayerID; current.Board.Edges[entry.Data] = e
		case "build_city":
			v := current.Board.Vertices[entry.Data]; v.OwnerID = entry.PlayerID; v.Type = "city"; current.Board.Vertices[entry.Data] = v
		}
		history = append(history, current.DeepCopy())
	}
	return history
}

func main() {
	loadTheme()
	topoData, _ := os.ReadFile("topology.yaml")
	var topo Topology
	yaml.Unmarshal(topoData, &topo)
	stateData, _ := os.ReadFile("game.yaml")
	var state GameState
	yaml.Unmarshal(stateData, &state)
	history := state.Replay(&topo)
	os.MkdirAll("vector_frames", 0755)
	for i, s := range history {
		filename := fmt.Sprintf("vector_frames/frame_%04d.png", i)
		renderVectorFrame(s, topo, i+1, len(history), filename)
	}
}
