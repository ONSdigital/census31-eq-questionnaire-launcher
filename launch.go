package main // import "github.com/ONSdigital/eq-questionnaire-launcher"

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ONSdigital/eq-questionnaire-launcher/authentication"
	"github.com/ONSdigital/eq-questionnaire-launcher/settings"
	"github.com/ONSdigital/eq-questionnaire-launcher/surveys"
	"github.com/gofrs/uuid"
	"github.com/gorilla/mux"
	"gopkg.in/square/go-jose.v2/json"
)

func randomNumericString(n int) string {
	var letter = []rune("0123456789")

	output := make([]rune, n)
	for i := range output {
		output[i] = letter[rand.Intn(len(letter))]
	}
	return string(output)
}

func serveTemplate(templateName string, data interface{}, w http.ResponseWriter, r *http.Request) {
	lp := filepath.Join("templates", "layout.html")
	fp := filepath.Join("templates", filepath.Clean(templateName))

	// Return a 404 if the template doesn't exist or is directory
	info, err := os.Stat(fp)
	if err != nil && (os.IsNotExist(err) || info.IsDir()) {
		log.Println("Cannot find: " + fp)
		http.NotFound(w, r)
		return
	}

	// Register custom template functions
	tmpl := template.New("layout.html").Funcs(template.FuncMap{
		"HasPrefix": strings.HasPrefix,
	})

	tmpl, err = tmpl.ParseFiles(lp, fp)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, http.StatusText(500), 500)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Println(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}

type page struct {
	Schemas                 map[string][]surveys.LauncherSchema
	CirSchemas              []surveys.CIMetadata
	AccountServiceURL       string
	AccountServiceLogOutURL string
	SdsEnabled              string
}

func getStatusPage(w http.ResponseWriter, r *http.Request) {
	_, writeError := w.Write([]byte("OK"))
	if writeError != nil {
		http.Error(w, fmt.Sprintf("Write failed to write data as part of an HTTP reply: %v", writeError), 500)
		return
	}
}

func getLaunchHandler(w http.ResponseWriter, r *http.Request) {
	p := page{
		Schemas:                 surveys.GetAvailableSchemas(),
		CirSchemas:              surveys.GetAvailableSchemasFromCIR(),
		AccountServiceURL:       getAccountServiceURL(r),
		AccountServiceLogOutURL: getAccountServiceURL(r),
		SdsEnabled:              settings.Get("SDS_ENABLED_IN_ENV"),
	}
	serveTemplate("launch.html", p, w, r)
}

func postLaunchHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("POST. r.ParseForm() err: %v", err), 500)
		return
	}
	redirectURL(w, r)
}

func getSurveyDataHandler(w http.ResponseWriter, r *http.Request) {
	schemaName := r.URL.Query().Get("schema_name")
	schemaUrl := r.URL.Query().Get("schema_url")
	cirInstrumentId := r.URL.Query().Get("cir_instrument_id")

	launcherSchema := surveys.GetLauncherSchema(schemaName, schemaUrl, cirInstrumentId)

	surveyData, err := authentication.GetSurveyData(launcherSchema)

	if err != "" {
		http.Error(w, fmt.Sprintf("GetSurveyData err: %v", err), 500)
		return
	}

	surveyDataJSON, _ := json.Marshal(surveyData)

	_, writeError := w.Write([]byte(surveyDataJSON))
	if writeError != nil {
		http.Error(w, fmt.Sprintf("Write failed to write data as part of an HTTP reply: %v", writeError), 500)
		return
	}
}

func getSupplementaryDataHandler(w http.ResponseWriter, r *http.Request) {
	surveyId := r.URL.Query().Get("survey_id")
	periodId := r.URL.Query().Get("period_id")
	sdsEnabled := settings.Get("SDS_ENABLED_IN_ENV")

	if sdsEnabled != "true" {
		return
	}

	datasets, err := surveys.GetSupplementaryDataSets(surveyId, periodId)
	if err != nil {
		http.Error(w, fmt.Sprintf("GetSupplementaryDataSets err: %v", err), 500)
		return
	}
	datasetJSON, _ := json.Marshal(datasets)

	_, writeError := w.Write([]byte(datasetJSON))
	if writeError != nil {
		http.Error(w, fmt.Sprintf("Write failed to write data as part of an HTTP reply: %v", writeError), 500)
		return
	}
}

func getAccountServiceURL(r *http.Request) string {
	forwardedProtocol := r.Header.Get("X-Forwarded-Proto")

	requestProtocol := "http"

	if forwardedProtocol != "" {
		requestProtocol = forwardedProtocol
	}

	return fmt.Sprintf("%s://%s",
		requestProtocol,
		html.EscapeString(r.Host))
}

func redirectURL(w http.ResponseWriter, r *http.Request) {
	hostURL := settings.Get("SURVEY_RUNNER_URL")

	launchVersion := r.FormValue("version")

	token := ""
	err := ""

	token, err = authentication.GenerateTokenFromPost(r.PostForm)

	if err != "" {
		http.Error(w, err, 500)
		return
	}

	flushAction := r.PostForm.Get("action_flush")
	log.Println("Request: " + r.PostForm.Encode())

	if flushAction != "" {
		http.Redirect(w, r, hostURL+"/flush?token="+token, 307)
	} else if launchVersion != "" {
		http.Redirect(w, r, hostURL+"/session?token="+token, 301)
	} else {
		http.Error(w, "Invalid Action", 500)
	}
}

func quickLauncherHandler(w http.ResponseWriter, r *http.Request) {
	hostURL := settings.Get("SURVEY_RUNNER_URL")
	accountServiceURL := getAccountServiceURL(r)
	urlValues := r.URL.Query()
	schemaURL := urlValues.Get("schema_url")

	defaultValues := authentication.GetDefaultValues()
	urlValues.Add("version", defaultValues["version"])

	log.Println("Quick launch request received", schemaURL)

	collectionExerciseSid, _ := uuid.NewV4()
	caseID, _ := uuid.NewV4()
	urlValues.Add("collection_exercise_sid", collectionExerciseSid.String())
	urlValues.Add("case_id", caseID.String())
	urlValues.Add("response_id", randomNumericString(16))
	urlValues.Add("language_code", defaultValues["language_code"])
	urlValues.Add("response_expires_at", time.Now().AddDate(0, 0, 7).Format("2006-01-02T15:04:05+00:00"))

	token := ""
	err := ""

	token, err = authentication.GenerateTokenFromDefaultsV2(schemaURL, accountServiceURL, urlValues)

	if err != "" {
		http.Error(w, err, 400)
		return
	}

	if schemaURL != "" {
		http.Redirect(w, r, hostURL+"/session?token="+token, 302)
	} else {
		http.Error(w, "Not Found", 404)
	}
}

func main() {
	r := mux.NewRouter()

	// Launch handlers
	r.HandleFunc("/", getLaunchHandler).Methods("GET")
	r.HandleFunc("/", postLaunchHandler).Methods("POST")
	r.HandleFunc("/survey-data", getSurveyDataHandler).Methods("GET")
	r.HandleFunc("/supplementary-data", getSupplementaryDataHandler).Methods("GET")

	//Author Launcher with passed parameters in Url
	r.HandleFunc("/quick-launch", quickLauncherHandler).Methods("GET")

	// Status Page
	r.HandleFunc("/status", getStatusPage).Methods("GET")

	// Serve static assets
	staticFs := http.FileServer(http.Dir("static"))
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", staticFs))

	// Bind to a port and pass our router in
	hostname := settings.Get("GO_LAUNCH_A_SURVEY_LISTEN_HOST") + ":" + settings.Get("GO_LAUNCH_A_SURVEY_LISTEN_PORT")

	log.Println("Listening on " + hostname)
	log.Fatal(http.ListenAndServe(hostname, r))
}
