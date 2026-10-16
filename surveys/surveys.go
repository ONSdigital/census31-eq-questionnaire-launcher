// Package surveys handles survey schema loading and management
package surveys

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sort"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/clients"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/settings"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// LauncherSchema is a representation of a schema in the Launcher
type LauncherSchema struct {
	Name       string
	SurveyType string
	URL        string
}

// GetAvailableSchemas Gets the list of static schemas
func GetAvailableSchemas() map[string][]LauncherSchema {
	runnerSchemas := getAvailableSchemasFromRunner()
	sort.Sort(ByFilename(runnerSchemas))

	schemasBySurveyType := map[string][]LauncherSchema{}
	for _, schema := range runnerSchemas {
		surveyType := cases.Title(language.Und).String(schema.SurveyType)
		schemasBySurveyType[surveyType] = append(schemasBySurveyType[surveyType], schema)
	}

	return schemasBySurveyType
}

// ByFilename implements sort.Interface based on the Name field.
type ByFilename []LauncherSchema

func (a ByFilename) Len() int           { return len(a) }
func (a ByFilename) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a ByFilename) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func getAvailableSchemasFromRunner() []LauncherSchema {

	schemaList := []LauncherSchema{}

	hostURL := settings.Get("SURVEY_RUNNER_SCHEMA_URL")

	log.Printf("Survey Runner Schema URL: %s", hostURL)

	url := fmt.Sprintf("%s/schemas", hostURL)

	resp, err := clients.GetHTTPClient().Get(url)

	if err != nil {
		return []LauncherSchema{}
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		return []LauncherSchema{}
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return []LauncherSchema{}
	}

	var schemaMapResponse = map[string][]string{}

	if err := json.Unmarshal(responseBody, &schemaMapResponse); err != nil {
		log.Print(err)
		return []LauncherSchema{}
	}

	for surveyType, schemas := range schemaMapResponse {
		for _, schemaName := range schemas {
			launcherSchema := LauncherSchema{
				Name:       schemaName,
				SurveyType: surveyType,
			}
			schemaList = append(schemaList, launcherSchema)
		}
	}

	return schemaList
}

// GetLauncherSchema returns a LauncherSchema struct with schema name or URL
func GetLauncherSchema(schemaName string, schemaURL string) LauncherSchema {
	var launcherSchema LauncherSchema

	switch {
	case schemaURL != "":
		log.Println("Getting schema by URL: " + schemaURL)
		launcherSchema = LauncherSchema{
			URL: schemaURL,
		}
	case schemaName != "":
		log.Println("Searching for schema by name: " + schemaName)
		launcherSchema = LauncherSchema{
			Name: schemaName,
		}
	default:
		panic("Either `schema_name` or `schema_url` must be provided.")
	}

	return launcherSchema
}
