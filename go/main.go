package main

import (
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

type services struct {
	Product   string   `json:"product"`
	Type      string   `json:"type"`
	Endpoints []string `json:"endpoints"`
}

var (
	listenAddress          = flag.String("web.listen-address", ":9800", "Address to listen on for telemetry")
	metricsPath            = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	resJSONServices        = "resources/services.json"
	externalServiceMapPath = "mapped-services"
	mapKeyEndpoint         map[string][]string
	mapKeyType             map[string][]string
)

// Readiness message
func ready() string {
	return "SA Exporter is ready to rock"
}

// Init of the exporter
func Init() *Exporter {
	flag.Parse()

	err := godotenv.Load(".env")
	if err != nil {
		log.Info(".env file absent, assume env variables are set.")
	}

	promEndpoint := os.Getenv("PROM_ENDPOINT")
	promUser := os.Getenv("PROMETHEUS_AUTH_USER")
	promPwd := os.Getenv("PROMETHEUS_AUTH_PWD")
	promLogin := ""
	if promUser == "" || promPwd == "" {
		log.Info("PROMETHEUS_AUTH_USER and or PROMETHEUS_AUTH_PWD were not set, will not use basic auth.")
	} else {
		promLogin = promUser + ":" + promPwd
	}

	saInteractiveAggr := os.Getenv("SA_INTERACTIVE_AGGR")
	saBatchAggr := os.Getenv("SA_BATCH_AGGR")

	// http://user:pass@localhost/ to use basic auth
	promURL := "http://" + promLogin + "@" + promEndpoint

	log.Info("sa Interactive Aggr => ", saInteractiveAggr)
	log.Info("sa Batch Aggr       => ", saBatchAggr)

	//populate services maps
	services := openServices(checkIfExternalServiceMap(externalServiceMapPath, resJSONServices))
	mapKeyType, mapKeyEndpoint = createServicesMaps(services)

	//Registering Exporter
	exporter := NewExporter(promURL, mapKeyType, mapKeyEndpoint, saInteractiveAggr, saBatchAggr)
	prometheus.MustRegister(exporter)

	return exporter
}

func checkIfExternalServiceMap(externalServicePath, defaultServiceJSON string) string {
	//will look at the external Service Path
	//if a json file is there then sa-exporter will consider this map instead of the default Service one

	files, err := os.ReadDir(externalServicePath)
	if err != nil {
		log.Error("checkIfExternalServiceMap not able to read dir for path ", externalServicePath, " err ", err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), "json") {
			newServiceMapFile := filepath.Join(externalServicePath, file.Name())
			log.Info("The file ", newServiceMapFile, " was found and will override the default service map.")
			return newServiceMapFile
		}
	}

	return defaultServiceJSON
}

func openServices(filename string) []services {
	jsonFile, err := os.Open(filename)
	if err != nil {
		log.Error(err)
		return nil
	}
	log.Info("Successfully Opened " + filename)

	defer jsonFile.Close()
	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		log.Error(err)
		return nil
	}
	var jsonServices []services
	json.Unmarshal(byteValue, &jsonServices)
	if len(jsonServices) == 0 {
		log.Error(filename, " is empty or not well formated")
	}
	for i := 0; i < len(jsonServices); i++ {
		log.Trace("Product: " + jsonServices[i].Product)
		log.Trace("Type: " + jsonServices[i].Type)
		log.Trace("Endpoints: " + jsonServices[i].Endpoints[0])
	}
	return jsonServices
}

func createServicesMaps(jsonServices []services) (map[string][]string, map[string][]string) {
	mapKeyEndpoint := make(map[string][]string)
	mapKeyType := make(map[string][]string)

	for i := 0; i < len(jsonServices); i++ {
		var typeEndpoint = jsonServices[i].Type
		//really weird syntax to add 2 slices with ... it is called "variadic"
		mapKeyType[typeEndpoint] = append(mapKeyType[typeEndpoint], jsonServices[i].Endpoints...)

		for j := 0; j < len(jsonServices[i].Endpoints); j++ {
			var endpoint = jsonServices[i].Endpoints[j]
			mapKeyEndpoint[endpoint] = append(mapKeyEndpoint[endpoint], jsonServices[i].Product)
		}
	}

	log.Info("Size maps, mapKeyType :", len(mapKeyType), " mapKeyEndpoint : ", len(mapKeyEndpoint))
	return mapKeyType, mapKeyEndpoint
}

func main() {
	log.Info("Starting SA exporter")
	Init()

	//This section will start the HTTP server and expose
	//any metrics on the /metrics endpoint.
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>SA Exporter</title></head>
             <body>
             <h1>` + ready() + `'</h1>
             <p><a href='` + *metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Info("Listening on port " + *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
