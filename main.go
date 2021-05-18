package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	svg "github.com/ajstarks/svgo"
)

func init() {
	rand.Seed(time.Now().Unix())
}

type ImageBundle struct {
	Data        []byte
	ContentType string
}

type Board [][]bool

func (b Board) Get(i, j int) bool {
	if i < 0 || i >= len(b) {
		return false
	}
	if j < 0 || j >= len(b[i]) {
		return false
	}
	return b[i][j]
}

func (b Board) String() string {
	s := ""
	for i := 0; i < len(b); i++ {
		for j := 0; j < len(b[i]); j++ {
			if b.Get(i, j) {
				s += "o"
			} else {
				s += "*"
			}
		}
		s += "\n"
	}
	return s
}

func (b Board) image(scale int) image.Image {
	k := scale
	img := image.NewRGBA(image.Rect(0, 0, k*len(b), k*len(b[0])))
	for i := 0; i < len(b); i++ {
		for j := 0; j < len(b[i]); j++ {
			if !b[i][j] {
				continue
			}
			draw.Draw(img, image.Rect(k*i, k*j, k*(i+1), k*(j+1)), &image.Uniform{C: color.RGBA{A: 255}}, image.Point{}, draw.Src)
		}
	}
	return img
}

func (b Board) Jpeg(scale int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, b.image(scale), nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (b Board) Png(scale int) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, b.image(scale)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (b Board) Svg(scale int) ([]byte, error) {
	k := scale
	var buf bytes.Buffer
	canvas := svg.New(&buf)
	canvas.Start(k*len(b), k*len(b[0]))
	for i := 0; i < len(b); i++ {
		for j := 0; j < len(b[i]); j++ {
			if !b[i][j] {
				continue
			}
			canvas.Rect(i*k, j*k, k, k, `fill="black"`)
		}
	}
	canvas.End()
	return buf.Bytes(), nil
}

func numberSvg(v int) []byte {
	s := strconv.Itoa(v)
	var buf bytes.Buffer
	canvas := svg.New(&buf)
	canvas.Start(len(s)*14, 14)
	canvas.Text(7, 7, strconv.Itoa(v),
		`font-size="14"`, `fill="red"`,
		`dominant-baseline="middle"`, `text-anchor="middle"`,
	)
	canvas.End()
	return buf.Bytes()
}

func NewBoard(w, h int) Board {
	b := make(Board, w)
	for i := 0; i < w; i++ {
		b[i] = make([]bool, h)
		for j := 0; j < h; j++ {
			if rand.Int()%5 == 0 {
				b[i][j] = true
			}
		}
	}
	return b
}

func Evolute(board Board) Board {
	newBoard := make(Board, len(board))
	for i := 0; i < len(board); i++ {
		newBoard[i] = make([]bool, len(board[i]))
		for j := 0; j < len(board[i]); j++ {
			var cnt int

			directions := [8][2]int{
				{1, 1},
				{1, 0},
				{1, -1},
				{0, 1},
				{0, -1},
				{-1, 1},
				{-1, 0},
				{-1, -1},
			}

			for _, dir := range directions {
				if board.Get(i+dir[0], j+dir[1]) {
					cnt++
				}
			}
			if cnt == 2 {
				newBoard[i][j] = board.Get(i, j)
			} else if cnt == 3 {
				newBoard[i][j] = true
			} else {
				newBoard[i][j] = false
			}
		}
	}
	return newBoard
}

type Render interface {
	Register(c chan<- ImageBundle) func()
}

type GameRender struct {
	gameChs map[chan<- ImageBundle]struct{}
}

func NewGameRender() Render {
	r := &GameRender{
		gameChs: make(map[chan<- ImageBundle]struct{}),
	}
	r.Start()
	return r
}

func (r *GameRender) Start() {
	go func() {
		b := NewBoard(80, 60)

		for {
			if len(r.gameChs) == 0 {
				time.Sleep(time.Millisecond * 100)
				continue
			}
			b = Evolute(b)

			img, err := b.Svg(10)
			if err != nil {
				fmt.Println(err)
				continue
			}
			bundle := ImageBundle{
				Data:        img,
				ContentType: "image/svg+xml",
			}

			for ch := range r.gameChs {
				select {
				case ch <- bundle:
				default:
				}
			}
			time.Sleep(time.Second)
		}
	}()
}

func (r *GameRender) Register(c chan<- ImageBundle) func() {
	r.gameChs[c] = struct{}{}
	return func() {
		delete(r.gameChs, c)
	}
}

type ViewersRender struct {
	viewerJoin  chan chan<- ImageBundle
	viewerLeave chan chan<- ImageBundle
}

func NewViewerRender() Render {
	r := &ViewersRender{
		viewerJoin:  make(chan chan<- ImageBundle),
		viewerLeave: make(chan chan<- ImageBundle),
	}
	r.Start()
	return r
}

func (r *ViewersRender) Start() {
	go func() {
		viewers := make(map[chan<- ImageBundle]struct{})

		for {
			select {
			case c := <-r.viewerJoin:
				viewers[c] = struct{}{}
			case c := <-r.viewerLeave:
				delete(viewers, c)
			case <-time.Tick(time.Second):
			}

			bundle := ImageBundle{
				Data:        numberSvg(len(viewers)),
				ContentType: "image/svg+xml",
			}
			for ch := range viewers {
				select {
				case ch <- bundle:
				default:
				}
			}
		}
	}()
}

func (r *ViewersRender) Register(c chan<- ImageBundle) func() {
	r.viewerJoin <- c
	return func() {
		r.viewerLeave <- c
	}
}

//go:embed index.html
var static embed.FS

func streamHandleFunc(render Render) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ch := make(chan ImageBundle)

		unregister := render.Register(ch)
		defer unregister()

		const boundary = "BOUNDARY"
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
		defer func() {
			w.Header().Set("Connection", "close")
		}()

		for {
			select {
			case <-r.Context().Done():
				return
			case data := <-ch:
				var err error
				_, err = w.Write([]byte("\r\n--" + boundary + "\r\n"))
				_, err = w.Write([]byte("Content-Type: " + data.ContentType + "\r\n"))
				_, err = w.Write([]byte("Content-Length: " + strconv.Itoa(len(data.Data)) + "\r\n\r\n"))
				_, err = w.Write(data.Data)
				if err != nil {
					fmt.Println(err)
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}
}

func main() {
	gameRender := NewGameRender()
	viewerRender := NewViewerRender()

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.HandleFunc("/game.svg", streamHandleFunc(gameRender))
	mux.HandleFunc("/viewers.svg", streamHandleFunc(viewerRender))
	log.Fatal(http.ListenAndServe(":3000", mux))
}
