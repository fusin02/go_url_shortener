package main

import (
	"database/sql"
	"flag"
	"fmt"
	"go_url_shortener/internals/models"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"

	_ "modernc.org/sqlite"
)

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

	err = template.Execute(w, pageData)
	if err != nil {
		fmt.Println(err.Error())
		serverError(w, err)
	}
}

func (a *App) routes() http.Handler {
	router := httprouter.New()
	fileServer := http.FileServer(http.Dir("./static/"))
	router.Handler(http.MethodGet, "/static/*filepath", http.StripPrefix("/static/", fileServer))

	router.HandlerFunc(http.MethodGet, "/", a.getDefaultRoute)

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
