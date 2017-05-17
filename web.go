package evepraisal

import (
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dustin/go-humanize"
	"github.com/elazarl/go-bindata-assetfs"
	"github.com/husobee/vestigo"
	"github.com/mash/go-accesslog"
)

type accessLogger struct {
}

func (l accessLogger) Log(record accesslog.LogRecord) {
	log.Printf("%s %s %d (%s) - %d", record.Method, record.Uri, record.Status, record.Ip, record.Size)
}

var templateFuncs = template.FuncMap{
	"commaf":          humanizeCommaf,
	"comma":           humanize.Comma,
	"prettybignumber": HumanLargeNumber,
	"relativetime":    humanize.Time,
	"timefmt":         func(t time.Time) string { return t.Format("2006-01-02 15:04:05") },

	// Only for debugging
	"spew": spew.Sdump,
}
var templates *template.Template

func init() {
	MustLoadTemplateFiles()
}

func MustLoadTemplateFiles() {
	t := template.New("root").Funcs(templateFuncs)
	for _, path := range AssetNames() {
		if strings.HasPrefix(path, "templates/") {
			tmpl := t.New(strings.TrimPrefix(path, "templates/"))
			fileContents, err := Asset(path)
			if err != nil {
				panic(err)
			}

			_, err = tmpl.Parse(string(fileContents))
			if err != nil {
				panic(err)
			}
		}
	}
	templates = t
}

type MainPageStruct struct {
	Appraisal           *Appraisal
	TotalAppraisalCount int64
}

func (app *App) HandleIndex(w http.ResponseWriter, r *http.Request) {
	txn := app.TransactionLogger.StartWebTransaction("view_index", w, r)
	defer txn.End()

	total, err := app.AppraisalDB.TotalAppraisals()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Something bad happened",
			ErrorMessage: err.Error(),
		})
		return
	}
	err = templates.ExecuteTemplate(w, "main.html", MainPageStruct{TotalAppraisalCount: total})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (app *App) HandleAppraisal(w http.ResponseWriter, r *http.Request) {
	txn := app.TransactionLogger.StartWebTransaction("create_appraisal", w, r)
	defer txn.End()

	log.Println("New appraisal at ", r.FormValue("market"))
	appraisal, err := app.StringToAppraisal(r.FormValue("market"), r.FormValue("body"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Invalid input",
			ErrorMessage: err.Error(),
		})
		return
	}

	err = app.AppraisalDB.PutNewAppraisal(appraisal)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Error when storing appraisal",
			ErrorMessage: err.Error(),
		})
		return
	}

	err = templates.ExecuteTemplate(
		w,
		"main.html",
		MainPageStruct{Appraisal: appraisal})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (app *App) HandleViewAppraisal(w http.ResponseWriter, r *http.Request) {
	txn := app.TransactionLogger.StartWebTransaction("view_appraisal", w, r)
	defer txn.End()

	appraisalID := vestigo.Param(r, "appraisalID")
	if strings.HasSuffix(appraisalID, ".json") {
		app.HandleViewAppraisalJSON(w, r)
		return
	}

	if strings.HasSuffix(appraisalID, ".raw") {
		app.HandleViewAppraisalRAW(w, r)
		return
	}

	appraisal, err := app.AppraisalDB.GetAppraisal(appraisalID)
	if err == AppraisalNotFound {
		w.WriteHeader(http.StatusNotFound)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Not Found",
			ErrorMessage: "I couldn't find what you're looking for",
		})
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Something bad happened",
			ErrorMessage: err.Error(),
		})
		return
	}

	sort.Slice(appraisal.Items, func(i, j int) bool {
		return appraisal.Items[i].SingleRepresentativePrice() > appraisal.Items[j].SingleRepresentativePrice()
	})

	err = templates.ExecuteTemplate(
		w,
		"main.html",
		MainPageStruct{Appraisal: appraisal})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (app *App) HandleViewAppraisalJSON(w http.ResponseWriter, r *http.Request) {
	txn := app.TransactionLogger.StartWebTransaction("view_appraisal_json", w, r)
	defer txn.End()

	appraisalID := vestigo.Param(r, "appraisalID")
	appraisalID = strings.TrimSuffix(appraisalID, ".json")

	appraisal, err := app.AppraisalDB.GetAppraisal(appraisalID)
	if err == AppraisalNotFound {
		w.WriteHeader(http.StatusNotFound)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Not Found",
			ErrorMessage: "I couldn't find what you're looking for",
		})
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Something bad happened",
			ErrorMessage: err.Error(),
		})
		return
	}

	r.Header["Content-Type"] = []string{"application/json"}
	json.NewEncoder(w).Encode(appraisal)
}

func (app *App) HandleViewAppraisalRAW(w http.ResponseWriter, r *http.Request) {
	txn := app.TransactionLogger.StartWebTransaction("view_appraisal_raw", w, r)
	defer txn.End()

	appraisalID := vestigo.Param(r, "appraisalID")
	appraisalID = strings.TrimSuffix(appraisalID, ".raw")

	appraisal, err := app.AppraisalDB.GetAppraisal(appraisalID)
	if err == AppraisalNotFound {
		w.WriteHeader(http.StatusNotFound)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Not Found",
			ErrorMessage: "I couldn't find what you're looking for",
		})
		return
	} else if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Something bad happened",
			ErrorMessage: err.Error(),
		})
		return
	}

	r.Header["Content-Type"] = []string{"application/text"}
	io.WriteString(w, appraisal.Raw)
}

func (app *App) HandleLatestAppraisals(w http.ResponseWriter, r *http.Request) {
	txn := app.TransactionLogger.StartWebTransaction("view_latest_appraisals", w, r)
	defer txn.End()

	appraisals, err := app.AppraisalDB.LatestAppraisals(100, "")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		templates.ExecuteTemplate(w, "error.html", ErrorPage{
			ErrorTitle:   "Something bad happened",
			ErrorMessage: err.Error(),
		})
		return
	}

	err = templates.ExecuteTemplate(
		w,
		"latest.html",
		struct{ Appraisals []Appraisal }{Appraisals: appraisals})

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type ErrorPage struct {
	ErrorTitle   string
	ErrorMessage string
}

func HTTPHandler(app *App) http.Handler {
	router := vestigo.NewRouter()
	router.Get("/latest", app.HandleLatestAppraisals)
	router.Get("/", app.HandleIndex)
	router.Post("/", app.HandleAppraisal)
	router.Get("/a/:appraisalID", app.HandleViewAppraisal)

	router.Handle("/expvar", expvar.Handler())

	vestigo.CustomNotFoundHandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			templates.ExecuteTemplate(w, "error.html", ErrorPage{
				ErrorTitle:   "Not Found",
				ErrorMessage: "I couldn't find what you're looking for",
			})
		})

	vestigo.CustomMethodNotAllowedHandlerFunc(func(allowedMethods string) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			templates.ExecuteTemplate(w, "error.html", ErrorPage{
				ErrorTitle:   "Method not allowed",
				ErrorMessage: fmt.Sprintf("HTTP Method not allowed. What is allowed is: " + allowedMethods),
			})
		}
	})

	mux := http.NewServeMux()

	// Route our bundled static files
	var fs = &assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo, Prefix: "/static/"}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))

	// Mount our web app router to root
	mux.Handle("/", router)

	return accesslog.NewLoggingHandler(mux, accessLogger{})
}
