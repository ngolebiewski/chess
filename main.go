package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math/rand"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"
)

//go:embed chess.png
var chessData []byte

const (
	tileSize = 16
	gridSize = 10
)

type Color int

const (
	Black Color = iota
	White
)

type PieceType int

const (
	Pawn PieceType = iota
	Bishop
	Rook
	Knight
	Queen
	King
)

// Piece values for Frank's brain
var pieceValues = map[PieceType]int{
	Pawn: 1, Knight: 3, Bishop: 3, Rook: 5, Queen: 9, King: 100,
}

type ChessPiece struct {
	Type     PieceType
	Color    Color
	SpriteID int
	HasMoved bool
}

var sprites []*ebiten.Image
var wallet = 100

type Game struct {
	board                [8][8]*ChessPiece
	selectedX, selectedY int
	activeColor          Color
	hustlerName          string
	currentDialog        string
	whiteTime, blackTime float64
	gameOver             bool
	gameStarted          bool
	wager                int
	initialMins          int
	rng                  *rand.Rand
	epX, epY             int
	winner               int
	moveCount            int
	frankThinkTime       int
	promoting            bool
	promX, promY         int
}

func NewGame(wager int, minutes int) *Game {
	g := &Game{
		selectedX: -1, selectedY: -1, epX: -1, epY: -1,
		activeColor:   White,
		hustlerName:   "4-Move-Frank",
		currentDialog: "Eyes on the board, kid.",
		whiteTime:     float64(minutes * 60 * 60),
		blackTime:     float64(minutes * 60 * 60),
		wager:         wager,
		gameStarted:   true,
		initialMins:   minutes,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
		winner:        -1,
	}
	g.setupBoard()
	return g
}

func (g *Game) setupBoard() {
	layout := []PieceType{Rook, Knight, Bishop, Queen, King, Bishop, Knight, Rook}
	for i := 0; i < 8; i++ {
		g.createPiece(layout[i], Black, i, 0)
		g.createPiece(Pawn, Black, i, 1)
		g.createPiece(Pawn, White, i, 6)
		g.createPiece(layout[i], White, i, 7)
	}
}

func (g *Game) createPiece(t PieceType, c Color, x, y int) {
	sid := int(t)
	if c == White {
		sid += 6
	}
	g.board[y][x] = &ChessPiece{Type: t, Color: c, SpriteID: sid, HasMoved: false}
}

func toAlg(x, y int) string { return fmt.Sprintf("%c%d", 'a'+x, 8-y) }
func pName(p *ChessPiece) string {
	return []string{"Pawn", "Bishop", "Rook", "Knight", "Queen", "King"}[p.Type]
}
func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (g *Game) isPathClear(fx, fy, tx, ty int) bool {
	dx, dy := tx-fx, ty-fy
	sx, sy := 0, 0
	if dx != 0 {
		sx = dx / abs(dx)
	}
	if dy != 0 {
		sy = dy / abs(dy)
	}
	cx, cy := fx+sx, fy+sy
	for cx != tx || cy != ty {
		if g.board[cy][cx] != nil {
			return false
		}
		cx += sx
		cy += sy
	}
	return true
}

func (g *Game) isMoveLegal(p *ChessPiece, fx, fy, tx, ty int) bool {
	if tx < 0 || tx > 7 || ty < 0 || ty > 7 {
		return false
	}
	target := g.board[ty][tx]
	if target != nil && target.Color == p.Color {
		return false
	}
	dx, dy := abs(tx-fx), abs(ty-fy)

	switch p.Type {
	case Knight:
		return (dx == 2 && dy == 1) || (dx == 1 && dy == 2)
	case Rook:
		return (fx == tx || fy == ty) && g.isPathClear(fx, fy, tx, ty)
	case Bishop:
		return dx == dy && g.isPathClear(fx, fy, tx, ty)
	case Queen:
		return (dx == dy || fx == tx || fy == ty) && g.isPathClear(fx, fy, tx, ty)
	case King:
		if dx <= 1 && dy <= 1 {
			return true
		}
		if p.HasMoved || fy != ty || dx != 2 || g.isInCheck(p.Color) {
			return false
		}
		rx := 0
		if tx > fx {
			rx = 7
		}
		rook := g.board[fy][rx]
		return rook != nil && rook.Type == Rook && !rook.HasMoved && g.isPathClear(fx, fy, rx, fy)
	case Pawn:
		dir := -1
		if p.Color == Black {
			dir = 1
		}
		if fx == tx && ty == fy+dir && target == nil {
			return true
		}
		if fx == tx && ty == fy+2*dir && fy == (map[Color]int{White: 6, Black: 1}[p.Color]) && target == nil && g.isPathClear(fx, fy, tx, ty) {
			return true
		}
		if dx == 1 && ty == fy+dir {
			if target != nil || (tx == g.epX && ty == g.epY) {
				return true
			}
		}
	}
	return false
}

func (g *Game) isInCheck(c Color) bool {
	kx, ky := -1, -1
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if p := g.board[y][x]; p != nil && p.Type == King && p.Color == c {
				kx, ky = x, y
			}
		}
	}
	if kx == -1 {
		return false
	}
	return g.isSquareAttacked(kx, ky, 1-c)
}

func (g *Game) isSquareAttacked(x, y int, attackerColor Color) bool {
	for fy := 0; fy < 8; fy++ {
		for fx := 0; fx < 8; fx++ {
			p := g.board[fy][fx]
			if p != nil && p.Color == attackerColor {
				if g.isMoveLegal(p, fx, fy, x, y) {
					return true
				}
			}
		}
	}
	return false
}

func (g *Game) hasLegalMoves(c Color) bool {
	for fy := 0; fy < 8; fy++ {
		for fx := 0; fx < 8; fx++ {
			p := g.board[fy][fx]
			if p == nil || p.Color != c {
				continue
			}
			for ty := 0; ty < 8; ty++ {
				for tx := 0; tx < 8; tx++ {
					if g.isMoveLegal(p, fx, fy, tx, ty) {
						orig := g.board[ty][tx]
						g.board[ty][tx], g.board[fy][fx] = p, nil
						safe := !g.isInCheck(c)
						g.board[fy][fx], g.board[ty][tx] = p, orig
						if safe {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (g *Game) executeMove(fx, fy, tx, ty int) {
	p := g.board[fy][fx]
	fmt.Printf("[%d] %s: %s -> %s\n", p.Color, pName(p), toAlg(fx, fy), toAlg(tx, ty))

	if p.Type == King && abs(tx-fx) == 2 {
		rx, rtx := 0, 3
		if tx > fx {
			rx, rtx = 7, 5
		}
		rook := g.board[fy][rx]
		g.board[fy][rtx], g.board[fy][rx] = rook, nil
		rook.HasMoved = true
	}

	if p.Type == Pawn && tx == g.epX && ty == g.epY {
		g.board[fy][tx] = nil
	}
	g.epX, g.epY = -1, -1
	if p.Type == Pawn && abs(ty-fy) == 2 {
		g.epX, g.epY = fx, fy+(ty-fy)/2
	}

	g.board[ty][tx], g.board[fy][fx] = p, nil
	p.HasMoved = true

	if p.Type == Pawn && (ty == 0 || ty == 7) {
		if p.Color == White {
			g.promoting = true
			g.promX, g.promY = tx, ty
		} else {
			p.Type = Queen
			p.SpriteID = 4
		}
	}
	if !g.promoting {
		g.activeColor = 1 - g.activeColor
	}
	g.moveCount++
	g.frankThinkTime = 0
}

func (g *Game) Update() error {
	if !g.gameStarted {
		if inpututil.IsKeyJustPressed(ebiten.Key1) {
			*g = *NewGame(5, 1)
		}
		if inpututil.IsKeyJustPressed(ebiten.Key2) {
			*g = *NewGame(50, 5)
		}
		return nil
	}
	if g.gameOver {
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			*g = *NewGame(g.wager, g.initialMins)
		}
		return nil
	}
	if g.promoting {
		if inpututil.IsKeyJustPressed(ebiten.KeyQ) {
			g.board[g.promY][g.promX].Type, g.board[g.promY][g.promX].SpriteID = Queen, 10
			g.promoting = false
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			g.board[g.promY][g.promX].Type, g.board[g.promY][g.promX].SpriteID = Rook, 8
			g.promoting = false
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyB) {
			g.board[g.promY][g.promX].Type, g.board[g.promY][g.promX].SpriteID = Bishop, 7
			g.promoting = false
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyN) {
			g.board[g.promY][g.promX].Type, g.board[g.promY][g.promX].SpriteID = Knight, 9
			g.promoting = false
		}
		if !g.promoting {
			g.activeColor = Black
		}
		return nil
	}
	if !g.hasLegalMoves(g.activeColor) {
		g.gameOver = true
		if g.isInCheck(g.activeColor) {
			if g.activeColor == White {
				g.winner, g.currentDialog = 0, "MATE! Give me my money."
				wallet -= g.wager
			} else {
				g.winner, g.currentDialog = 1, "MATE! Take the cash."
				wallet += g.wager
			}
		}
		return nil
	}

	if g.activeColor == White {
		g.whiteTime--
		if g.whiteTime <= 0 {
			g.gameOver, g.winner, wallet = true, 0, wallet-g.wager
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			mx, my := ebiten.CursorPosition()
			gx, gy := (mx/tileSize)-1, (my/tileSize)-1
			if gx >= 0 && gx < 8 && gy >= 0 && gy < 8 {
				if g.selectedX == -1 {
					if g.board[gy][gx] != nil && g.board[gy][gx].Color == White {
						g.selectedX, g.selectedY = gx, gy
					}
				} else {
					p := g.board[g.selectedY][g.selectedX]
					if g.isMoveLegal(p, g.selectedX, g.selectedY, gx, gy) {
						orig := g.board[gy][gx]
						g.board[gy][gx], g.board[g.selectedY][g.selectedX] = p, nil
						if !g.isInCheck(White) {
							g.board[g.selectedY][g.selectedX], g.board[gy][gx] = p, orig
							g.executeMove(g.selectedX, g.selectedY, gx, gy)
						} else {
							g.board[g.selectedY][g.selectedX], g.board[gy][gx] = p, orig
						}
					}
					g.selectedX, g.selectedY = -1, -1
				}
			}
		}
	} else {
		g.blackTime--
		if g.blackTime <= 0 {
			g.gameOver, g.winner, wallet = true, 1, wallet+g.wager
		}
		g.frankThinkTime++
		limit := 180
		if g.moveCount < 6 {
			limit = 60
		} else if g.moveCount < 16 {
			limit = 120
		}
		if g.frankThinkTime >= limit {
			// SCHOLAR'S MATE (STILL PRIORITIZED)
			script := [][]int{{4, 1, 4, 3}, {3, 0, 7, 4}, {5, 0, 2, 3}, {7, 4, 5, 6}}
			for _, m := range script {
				if p := g.board[m[1]][m[0]]; p != nil && p.Color == Black && g.isMoveLegal(p, m[0], m[1], m[2], m[3]) {
					orig := g.board[m[3]][m[2]]
					g.board[m[3]][m[2]], g.board[m[1]][m[0]] = p, nil
					if !g.isInCheck(Black) {
						g.board[m[1]][m[0]], g.board[m[3]][m[2]] = p, orig
						g.executeMove(m[0], m[1], m[2], m[3])
						return nil
					}
					g.board[m[1]][m[0]], g.board[m[3]][m[2]] = p, orig
				}
			}

			type move struct {
				fx, fy, tx, ty int
				score          int
			}
			var smartMoves []move
			for fy := 0; fy < 8; fy++ {
				for fx := 0; fx < 8; fx++ {
					if p := g.board[fy][fx]; p != nil && p.Color == Black {
						for ty := 0; ty < 8; ty++ {
							for tx := 0; tx < 8; tx++ {
								if g.isMoveLegal(p, fx, fy, tx, ty) {
									orig := g.board[ty][tx]

									// CALC SCORE
									score := 0
									// Is the piece currently in danger?
									inDanger := g.isSquareAttacked(fx, fy, White)
									if inDanger {
										score += pieceValues[p.Type] * 2 // Incentive to move piece out of danger
									}
									// Attack/Capture value
									if orig != nil {
										score += pieceValues[orig.Type] + 2
									}

									// TEST MOVE
									g.board[ty][tx], g.board[fy][fx] = p, nil
									if !g.isInCheck(Black) {
										// Penalty for moving INTO danger
										if g.isSquareAttacked(tx, ty, White) {
											score -= pieceValues[p.Type] + 1
										}
										smartMoves = append(smartMoves, move{fx, fy, tx, ty, score})
									}
									g.board[fy][fx], g.board[ty][tx] = p, orig
								}
							}
						}
					}
				}
			}

			if len(smartMoves) > 0 {
				best := smartMoves[0]
				for _, m := range smartMoves {
					if m.score > best.score {
						best = m
					}
				}
				g.executeMove(best.fx, best.fy, best.tx, best.ty)
			}
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	if !g.gameStarted {
		text.Draw(screen, "CHOOSE STAKES:", basicfont.Face7x13, 20, 60, color.White)
		text.Draw(screen, "1: $5 Bullet | 2: $50 Blitz", basicfont.Face7x13, 20, 90, color.RGBA{0, 255, 150, 255})
		text.Draw(screen, fmt.Sprintf("WALLET: $%d", wallet), basicfont.Face7x13, 20, 120, color.RGBA{255, 215, 0, 255})
		return
	}
	for y := 0; y < gridSize; y++ {
		for x := 0; x < gridSize; x++ {
			px, py := float64(x*tileSize), float64(y*tileSize)
			if x == 0 || x == 9 || y == 0 || y == 9 {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(px, py)
				screen.DrawImage(sprites[14], op)
			} else {
				bx, by := x-1, y-1
				tID := 13
				if (bx+by)%2 != 0 {
					tID = 12
				}
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(px, py)
				if bx == g.selectedX && by == g.selectedY {
					op.ColorScale.Scale(2, 0.5, 0.5, 1)
				}
				screen.DrawImage(sprites[tID], op)
				if p := g.board[by][bx]; p != nil {
					pop := &ebiten.DrawImageOptions{}
					pop.GeoM.Translate(px, py)
					screen.DrawImage(sprites[p.SpriteID], pop)
				}
			}
		}
	}
	dy := float32(gridSize * tileSize)
	vector.FillRect(screen, 0, dy, 160, 40, color.RGBA{10, 10, 15, 255}, false)
	text.Draw(screen, fmt.Sprintf("W:%02d:%02d B:%02d:%02d", int(g.whiteTime/3600), int(g.whiteTime/60)%60, int(g.blackTime/3600), int(g.blackTime/60)%60), basicfont.Face7x13, 5, int(dy)+12, color.White)
	text.Draw(screen, fmt.Sprintf("STAKES:$%d WALLET:$%d", g.wager, wallet), basicfont.Face7x13, 5, int(dy)+24, color.RGBA{255, 215, 0, 255})
	msg := g.hustlerName + ": " + g.currentDialog
	if g.activeColor == Black && !g.gameOver {
		msg = g.hustlerName + ": ..."
	}
	text.Draw(screen, msg, basicfont.Face7x13, 5, int(dy)+36, color.White)
	if g.promoting {
		vector.FillRect(screen, 10, 40, 140, 80, color.RGBA{0, 0, 0, 230}, false)
		text.Draw(screen, "PROMOTE: Q R B N", basicfont.Face7x13, 25, 80, color.White)
	}
	if g.gameOver {
		vector.FillRect(screen, 0, 50, 160, 60, color.RGBA{0, 0, 0, 240}, false)
		win := "FRANK WINS"
		if g.winner == 1 {
			win = "YOU WIN!"
		}
		text.Draw(screen, "CHECKMATE!", basicfont.Face7x13, 45, 75, color.RGBA{255, 50, 50, 255})
		text.Draw(screen, win, basicfont.Face7x13, 45, 95, color.White)
	}
}

func (g *Game) Layout(w, h int) (int, int) { return 160, 200 }

func main() {
	img, _, _ := image.Decode(bytes.NewReader(chessData))
	sheet := ebiten.NewImageFromImage(img)
	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			r := image.Rect(x*tileSize, y*tileSize, (x+1)*tileSize, (y+1)*tileSize)
			sprites = append(sprites, sheet.SubImage(r).(*ebiten.Image))
		}
	}
	ebiten.SetWindowSize(640, 800)
	ebiten.RunGame(&Game{gameStarted: false})
}
