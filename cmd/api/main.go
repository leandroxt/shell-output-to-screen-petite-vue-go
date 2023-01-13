package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var conn *websocket.Conn

type application struct {
	log struct {
		inf *log.Logger
		err *log.Logger
	}
}

func (app *application) listFiles(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("bad server"))
		return
	}

	app.log.inf.Printf("Path %s", input.Path)

	go func() {
		app.log.inf.Printf("Starting async")

		var out bytes.Buffer
		cmd := exec.Command("sh", "./cmd/sh/execute.sh", input.Path)
		cmd.Stdout = &out
		err := cmd.Run()

		if err != nil {
			app.log.err.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err = conn.WriteMessage(1, out.Bytes()); err != nil {
			app.log.err.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		app.log.inf.Printf("%s", out.String())
	}()

	w.WriteHeader(http.StatusAccepted)
	w.Header().Set("Content-type", "application/json")
	w.Write([]byte("{\"path\":\"" + input.Path + "\"}"))
}

func (app *application) webSocket(w http.ResponseWriter, r *http.Request) {
	app.upgradeConnection(w, r)
}

func (app *application) upgradeConnection(w http.ResponseWriter, r *http.Request) *websocket.Conn {
	var err error
	conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.log.inf.Print(err)

		conn.Close()
	}

	return conn
}

func main() {
	var addr int
	flag.IntVar(&addr, "port", 8080, "API server port")
	flag.Parse()

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	app := &application{}
	app.log.inf = infoLog
	app.log.err = errorLog

	router := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./static"))
	router.Handle("/", http.StripPrefix("/", fileServer))
	router.HandleFunc("/list", app.listFiles)
	router.HandleFunc("/ws", app.webSocket)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", addr),
		Handler:      app.recoverPanic(router),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	infoLog.Printf("Starting server on %d", addr)
	err := server.ListenAndServe()
	errorLog.Fatal(err)
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
