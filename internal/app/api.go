package app

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/url"

	"github.com/beriholic/segra/internal/server"
)

func (a *App) startAPI() {
	a.srv = server.New(a.mgr, a.settings.APIToken, a.settings.APIPort)
	port, err := a.srv.Start()
	if err != nil {
		a.apiErr = err.Error()
		log.Printf("api: %v", err)
		return
	}
	a.apiPort = port
	a.apiErr = ""
	log.Printf("api listening on 127.0.0.1:%d", port)
}

func (a *App) restartAPI() {
	a.stopAPI()
	a.startAPI()
}

// stopAPI closes the local API, leaving no port for the extension to reach.
func (a *App) stopAPI() {
	if a.srv != nil {
		_ = a.srv.Stop()
		a.srv = nil
	}
	a.apiPort = 0
	a.apiErr = ""
}

func newToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}

func (a *App) openDeepLink(raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	link := u.Query().Get("url")
	if link == "" && u.Host != "" && u.Host != "download" {
		link = "https://" + u.Host + u.Path
	}
	if link == "" {
		return
	}
	a.openNew()
	a.newLink = link
	a.showWindow()
	a.invalidate()
}
