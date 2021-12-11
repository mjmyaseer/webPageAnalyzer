package main

import (
	"crypto/tls"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"github.com/sclevine/agouti"
	"golang.org/x/net/html"
	"golang.org/x/net/websocket"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var driver *agouti.WebDriver

func init() {
	driver = agouti.ChromeDriver(
		agouti.ChromeOptions("args", []string{
			"--headless",
			"--window-size=1680,1050",
			"--no-sandbox",
			"--disable-gpu",
		}),
	)
	err := driver.Start()
	if err != nil {
		log.Printf("Failed to start driver. please restart server: %v", err)
		os.Exit(1)
	}
}

func getHTML(url string) (string, error) {
	page, err := driver.NewPage(agouti.Browser("chrome"))
	if err != nil {
		return "", errors.Wrap(err, "Failed to open page")
	}

	err = page.Navigate(url)
	if err != nil {
		return "", errors.Wrap(err, "Failed to Navigate")
	}

	content, err := page.HTML()
	if err != nil {
		return "", errors.Wrap(err, "Failed to get html")
	}

	return content, nil
}

func getDocument(html string) (*goquery.Document, error) {
	reader := strings.NewReader(html)

	doc, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get document")
	}

	return doc, nil
}

func getEnv(key, defaultValue string) string {
	env := os.Getenv(key)
	if env == "" {
		return defaultValue
	}
	return env
}

func webSocketHost() string {
	return getEnv("ANALYZER_WEBSOCKET_HOST", "localhost")
}

func webSocketPort() string {
	return getEnv("ANALYZER_WEBSOCKET_PORT", "8080")
}

func index(w http.ResponseWriter, _ *http.Request) {
	params := map[string]string{
		"WebSocketHost": webSocketHost(),
		"WebSocketPort": webSocketPort(),
	}

	t := template.Must(template.ParseFiles("view/index.html.tpl"))
	if err := t.ExecuteTemplate(w, "index.html.tpl", params); err != nil {
		log.Printf("Failed to parse view: %v", err)
	}
}

func websocketHandler(ws *websocket.Conn) {
	for {
		var err error
		var url string

		if err = websocket.Message.Receive(ws, &url); err != nil {
			log.Printf("couldn't receive websocket message %v", err)
			break
		}

		_, err = NewHTTPClient().Get(url)
		if err != nil {
			ResponseFailure(ws, err.Error())
			continue
		}

		rawHTML, err := getHTML(url)
		if err != nil {
			ResponseFailure(ws, err.Error())
			continue
		}

		document, err := getDocument(rawHTML)
		if err != nil {
			ResponseFailure(ws, err.Error())
			continue
		}

		analyzer := NewAnalyzer(ws, url, rawHTML, document)
		analyzer.Start()
		analyzer.Wait()
		analyzer.Complete()
	}
}

func main() {
	defer func(driver *agouti.WebDriver) {
		err := driver.Stop()
		if err != nil {
			log.Printf("Failed to stop the service. please contact admin: %v", err)
			os.Exit(1)
		}
	}(driver)
	http.HandleFunc("/", index)
	http.Handle("/webSocket", websocket.Handler(websocketHandler))
	if err := http.ListenAndServe(fmt.Sprintf(":%s", webSocketPort()), nil); err != nil {
		log.Printf("Failed to start the service. please contact admin: %v", err)
		os.Exit(1)
	}
}

func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}

type analyzeResponseStatus int

const (
	statusSuccess analyzeResponseStatus = iota
	statusFailure
	statusComplete
)

type analyzeResponse struct {
	Result string
	Status analyzeResponseStatus
}

// ResponseSuccess returns success response to client.
func ResponseSuccess(ws *websocket.Conn, message string) {
	writeResponse(ws, message, statusSuccess)
}

// ResponseFailure returns failure response to client.
func ResponseFailure(ws *websocket.Conn, message string) {
	writeResponse(ws, message, statusFailure)
}

// ResponseComplete returns complete response to client.
func ResponseComplete(ws *websocket.Conn, message string) {
	writeResponse(ws, message, statusComplete)
}

func writeResponse(ws *websocket.Conn, message string, status analyzeResponseStatus) {
	if err := websocket.JSON.Send(ws, analyzeResponse{Result: message, Status: status}); err != nil {
		log.Printf("couldn't send websocket response %v", err)
	}
}

// Analyzer represents analyzer of web pages.
type Analyzer struct {
	waitGroup  *sync.WaitGroup
	ws         *websocket.Conn
	requestURL string
	rawHTML    string
	document   *goquery.Document

	internalLink int
	externalLink int

	startTime      time.Time
	processingTime time.Duration
}

// NewAnalyzer returns new Analyzer.
func NewAnalyzer(ws *websocket.Conn,
	requestURL string,
	rawHTML string,
	document *goquery.Document) *Analyzer {

	return &Analyzer{
		ws:         ws,
		rawHTML:    rawHTML,
		document:   document,
		requestURL: requestURL,
		waitGroup:  &sync.WaitGroup{},
	}
}

// Start starts analyzing web page.
func (a *Analyzer) Start() {
	a.startTime = time.Now()
	a.concur(a.findTitle)
	a.concur(a.findDocType)
	for i := 1; i <= 6; i++ {
		a.concur(a.findHeading(i))
	}
	a.concur(a.findLinks)
	a.concur(a.findLoginForm)
}

// Wait waits until end of analyzing web page.
func (a *Analyzer) Wait() {
	a.waitGroup.Wait()
	a.processingTime = time.Since(a.startTime)
}

// Complete sends response of complete of analyzing web page to client.
func (a *Analyzer) Complete() {
	ResponseComplete(a.ws, fmt.Sprintf("analyzing completed : total processing time %s", a.processingTime))
}

func (a *Analyzer) concur(f func()) {
	a.waitGroup.Add(1)
	go func() {
		defer a.waitGroup.Done()
		f()
	}()
}

func (a *Analyzer) findDocType() {
	firstline := strings.Split(a.rawHTML, "\n")[0]
	r, _ := regexp.Compile("<!DOCTYPE(.*?)>")
	match := r.FindString(firstline)
	ResponseSuccess(a.ws, fmt.Sprintf("html version : %s", html.EscapeString(match)))
}

func (a *Analyzer) findTitle() {
	value := a.document.Find("title").Text()
	ResponseSuccess(a.ws, fmt.Sprintf("title : %s", html.EscapeString(value)))
}

func (a *Analyzer) findHeading(level int) func() {
	return func() {
		var value int
		findLevel := fmt.Sprintf("h%d", level)
		a.document.Find(findLevel).Each(func(_ int, _ *goquery.Selection) { value++ })
		ResponseSuccess(a.ws, fmt.Sprintf("%s count : %d", findLevel, value))
	}
}

func (a *Analyzer) findLinks() {
	ignoreList := map[string]bool{}

	a.document.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		if ignoreList[link] {
			return
		}

		ignoreList[link] = true

		parsedURL, err := url.ParseRequestURI(link)
		if err != nil {
			return
		}

		if parsedURL.Host == "" {
			a.internalLink++
		} else {
			a.externalLink++
		}
	})

	ResponseSuccess(a.ws, fmt.Sprintf("internal link count : %d", a.internalLink))
	ResponseSuccess(a.ws, fmt.Sprintf("external link count : %d", a.externalLink))
}

func (a *Analyzer) findLoginForm() {
	var loginFound bool
	a.document.Find("form").Each(func(_ int, s *goquery.Selection) {
		action, _ := s.Attr("action")
		if strings.Contains(action, "login") {
			loginFound = true
		}
	})
	ResponseSuccess(a.ws, fmt.Sprintf("contain login form : %s", strconv.FormatBool(loginFound)))
}
