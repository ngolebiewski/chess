package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

//go:embed chess.png
var chessData []byte

const (
	tileSize  = 16
	boardSize = 8
	gridSize  = 10 // 8 + 2 for borders
)

// --- Logic Types ---

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

type ChessPiece struct {
	Type     PieceType
	Color    Color
	Value    int
	SpriteID int
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (p *ChessPiece) IsMoveLegal(fromX, fromY, toX, toY int) bool {
	dx := abs(toX - fromX)
	dy := abs(toY - fromY)

	switch p.Type {
	case Knight:
		return (dx == 2 && dy == 1) || (dx == 1 && dy == 2)
	default:
		return true
	}
}

// --- Game Engine ---

var sprites []*ebiten.Image

type Game struct {
	board       [boardSize][boardSize]*ChessPiece
	playerColor Color
}

func (g *Game) createPiece(pType PieceType, pColor Color, x, y int) {
	valMap := map[PieceType]int{Pawn: 1, Knight: 3, Bishop: 3, Rook: 5, Queen: 9, King: 0}
	spriteID := int(pType)
	if pColor == White {
		spriteID += 6
	}
	g.board[y][x] = &ChessPiece{
		Type:     pType,
		Color:    pColor,
		Value:    valMap[pType],
		SpriteID: spriteID,
	}
}

func NewGame(pColor Color) *Game {
	g := &Game{playerColor: pColor}

	// Standard layout (D-file Queen, E-file King)
	// We adjust the order for Black perspective so Queen stays on her color
	whiteLayout := []PieceType{Rook, Knight, Bishop, Queen, King, Bishop, Knight, Rook}
	blackLayout := []PieceType{Rook, Knight, Bishop, Queen, King, Bishop, Knight, Rook}

	if pColor == White {
		// White is bottom: Row 0 is Top (Black), Row 7 is Bottom (White)
		for i := 0; i < 8; i++ {
			g.createPiece(blackLayout[i], Black, i, 0)
			g.createPiece(Pawn, Black, i, 1)
			g.createPiece(Pawn, White, i, 6)
			g.createPiece(whiteLayout[i], White, i, 7)
		}
	} else {
		// Black is bottom: Row 0 is Top (White), Row 7 is Bottom (Black)
		for i := 0; i < 8; i++ {
			g.createPiece(whiteLayout[i], White, i, 0)
			g.createPiece(Pawn, White, i, 1)
			g.createPiece(Pawn, Black, i, 6)
			g.createPiece(blackLayout[i], Black, i, 7)
		}
	}
	return g
}

func (g *Game) Update() error { return nil }

func (g *Game) Draw(screen *ebiten.Image) {
	// 1. Draw the 10x10 Grid (Border + Board)
	for y := 0; y < gridSize; y++ {
		for x := 0; x < gridSize; x++ {
			posX := float64(x * tileSize)
			posY := float64(y * tileSize)

			// Check if we are on the outer edge (0 or 9)
			if x == 0 || x == gridSize-1 || y == 0 || y == gridSize-1 {
				// DRAW BORDER
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(posX, posY)
				screen.DrawImage(sprites[14], op)
			} else {
				// DRAW BOARD SQUARE (Internal 8x8)
				// Adjust board indices to 0-7 by subtracting 1
				bx, by := x-1, y-1

				tileID := 13 // White
				if (bx+by)%2 != 0 {
					tileID = 12 // Black
				}

				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(posX, posY)
				screen.DrawImage(sprites[tileID], op)

				// DRAW PIECE
				if piece := g.board[by][bx]; piece != nil {
					pop := &ebiten.DrawImageOptions{}
					pop.GeoM.Translate(posX, posY)
					screen.DrawImage(sprites[piece.SpriteID], pop)
				}
			}
		}
	}

	// 2. Draw Coordinate Labels on top of the border
	for i := 0; i < 8; i++ {
		var colLabel, rowLabel string
		if g.playerColor == White {
			colLabel = string('a' + rune(i))
			rowLabel = fmt.Sprint(8 - i)
		} else {
			colLabel = string('h' - rune(i))
			rowLabel = fmt.Sprint(1 + i)
		}

		// Text positioning (shifted slightly to center in the 16x16 border tiles)
		text.Draw(screen, colLabel, basicfont.Face7x13, (i+1)*tileSize+5, 12, color.White)
		text.Draw(screen, colLabel, basicfont.Face7x13, (i+1)*tileSize+5, 9*tileSize+12, color.White)
		text.Draw(screen, rowLabel, basicfont.Face7x13, 5, (i+1)*tileSize+12, color.White)
		text.Draw(screen, rowLabel, basicfont.Face7x13, 9*tileSize+5, (i+1)*tileSize+12, color.White)
	}
}

func (g *Game) Layout(w, h int) (int, int) {
	return gridSize * tileSize, gridSize * tileSize
}

func main() {
	img, _, err := image.Decode(bytes.NewReader(chessData))
	if err != nil {
		log.Fatal(err)
	}
	fullSheet := ebiten.NewImageFromImage(img)

	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			rect := image.Rect(x*tileSize, y*tileSize, (x+1)*tileSize, (y+1)*tileSize)
			sprites = append(sprites, fullSheet.SubImage(rect).(*ebiten.Image))
		}
	}

	ebiten.SetWindowSize(640, 640)
	ebiten.SetWindowTitle("Chess")
	if err := ebiten.RunGame(NewGame(White)); err != nil {
		log.Fatal(err)
	}
}
