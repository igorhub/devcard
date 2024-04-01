package server

import (
	_ "embed"
	"strings"
)

//go:embed assets/head.html
var headHTML string

//go:embed assets/body.html
var bodyHTML string

//go:embed assets/javascript.js
var javascript string

type page struct {
	title       string
	clientId    string
	clientKind  string
	url         string
	projectName string
	devcardName string
	body        string
}

func (p page) generate() []byte {
	html := headHTML
	body := bodyHTML
	if p.body != "" {
		body = p.body
	}
	html = strings.ReplaceAll(html, "{{body}}", body)
	html = strings.ReplaceAll(html, "{{javascript}}", javascript)
	html = strings.ReplaceAll(html, "{{title}}", p.title)
	html = strings.ReplaceAll(html, "{{clientId}}", p.clientId)
	html = strings.ReplaceAll(html, "{{clientKind}}", p.clientKind)
	html = strings.ReplaceAll(html, "{{url}}", p.url)
	html = strings.ReplaceAll(html, "{{projectName}}", p.projectName)
	html = strings.ReplaceAll(html, "{{devcardName}}", p.devcardName)
	return []byte(html)
}
