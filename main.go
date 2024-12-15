package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"flag"
	"fmt"
	"go_url_shortener/internals/models"
	"html/template"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	urlverifier "github.com/davidmytton/url-verifier"
	"github.com/gorilla/sessions"
	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"

	_ "modernc.org/sqlite"
)

func uniqueId(prefix string) string {
	now := time.Now()
	sec := now.Unix()
	usec := now.UnixNano() % 0x100000
	return fmt.Sprintf("%s%08x%05x", prefix, sec, usec)
}

func (a *App) GenerateShortenedUrl() string {
	var (
		randomChars   = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
		randIntLength = 27
		stringLength  = 32
	)

	str := make([]rune, stringLength)

	for char := range str {
		nBig, err := rand.Int(rand.Reader, big.NewInt(int64(randIntLength)))
		if err != nil {
			panic(err)
		}
		str[char] = randomChars[nBig.Int64()]
	}

	hash := sha256.Sum256([]byte(uniqueId(string(str))))
	encodedString := base64.StdEncoding.EncodeToString(hash[:])

	return encodedString[0:9]
}

func setErrorInFlash(error string, w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "flash-messages")
	if err != nil {
		fmt.Println(err.Error())
	}
	session.AddFlash(error, "error")
	session.Save(r, w)
}

var store = sessions.NewCookieStore([]byte("My super secret authentication key"))

type PageData struct {
	BaseUrl, Error string
	UrlData        []*models.Url
}

type App struct {
	urls *models.ShortenerDataModel
}

func serverError(w http.ResponseWriter, err error) {
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func newApp(dbPath string) App {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	return App{urls: &models.ShortenerDataModel{DB: db}}
}

var functions = template.FuncMap{
	"formatClicks": formatClicks,
}

func formatClicks(clicks int) string {
	p := message.NewPrinter(language.English)
	return p.Sprintf("%d", number.Decimal(clicks))
}

func (a *App) getDefaultRoute(w http.ResponseWriter, r *http.Request) {
	templatePath := "templates/default.html"
	template, err := template.New("default.html").Funcs(functions).ParseFiles(templatePath)
	if err != nil {
		fmt.Println(err.Error())
		serverError(w, err)
		return
	}

	urls, err := a.urls.GetLatest()
	if err != nil {
		fmt.Printf("Error getting latest urls: %s\n", err.Error())
		return
	}

	baseUrl := "http://" + r.Host + "/"
	pageData := PageData{BaseUrl: baseUrl, UrlData: urls}

	session, err := store.Get(r, "flash-session")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fm := session.Flashes("error")
	if fm != nil {
		if error, ok := fm[0].(string); ok {
			pageData.Error = error
		} else {
			fmt.Printf("Session flash did not contain an error message. Contained %s. \n", fm[0])
		}
	}
	session.Save(r, w)

	err = template.Execute(w, pageData)
	if err != nil {
		fmt.Println(err.Error())
		serverError(w, err)
	}
}

func (a *App) shortenUrl(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		fmt.Println(err.Error())
		serverError(w, err)
		return
	}

	originalUrl := r.PostForm.Get("url")
	if originalUrl == "" {
		setErrorInFlash("URL cannot be empty", w, r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	verifier := urlverifier.NewVerifier()
	verifier.EnableHTTPCheck()
	result, err := verifier.Verify(originalUrl)

	if err != nil {
		fmt.Println(err.Error())
		setErrorInFlash(err.Error(), w, r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !result.IsURL {
		fmt.Printf("[%s] is not a valid URL! \n", originalUrl)
		setErrorInFlash("Sorry! I can only shorten valid URLs.", w, r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !result.HTTP.Reachable {
		fmt.Printf("[%s] was not reachable! \n", originalUrl)
		setErrorInFlash("Sorry! The URL was not reachable.", w, r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	shortenedUrl := a.GenerateShortenedUrl()
	_, err = a.urls.Insert(originalUrl, shortenedUrl, 0) // Saves the URL to the database
	if err != nil {
		fmt.Println(err.Error())
		setErrorInFlash("Error shortening URL", w, r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	fmt.Printf("Redirecting [%s] to [%s]\n", originalUrl, shortenedUrl)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) openShortenedRoute(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())
	shortened := params.ByName("url")

	original, err := a.urls.Get(shortened)
	if err != nil {
		fmt.Println(err.Error())
		serverError(w, err)
		return
	}

	err = a.urls.UpdateClicks(shortened)
	if err != nil {
		fmt.Println(err.Error())
		serverError(w, err)
		return
	}

	http.Redirect(w, r, original, http.StatusSeeOther)
}

func (a *App) routes() http.Handler {
	router := httprouter.New()
	fileServer := http.FileServer(http.Dir("./static/"))
	router.Handler(http.MethodGet, "/static/*filepath", http.StripPrefix("/static/", fileServer))

	router.HandlerFunc(http.MethodGet, "/", a.getDefaultRoute)
	router.HandlerFunc(http.MethodPost, "/", a.shortenUrl)
	router.HandlerFunc(http.MethodGet, "/o/:url", a.openShortenedRoute)

	standard := alice.New()

	return standard.Then(router)
}

func main() {
	app := newApp("data/database.sqlite3")
	addr := flag.String("addr", ":8080", "HTTP network address")

	infoLog := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog := log.New(os.Stderr, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	defer app.urls.DB.Close()

	srv := &http.Server{
		Addr:     *addr,
		ErrorLog: errorLog,
		Handler:  app.routes(),
	}

	infoLog.Printf("Starting server on %s", *addr)
	err := srv.ListenAndServe()
	errorLog.Fatal(err)
}
