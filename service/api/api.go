package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/exporter/trace/stdout"
	"go.opentelemetry.io/otel/plugin/httptrace"
	"go.opentelemetry.io/otel/plugin/othttp"
)

var services Config

func Start() {
	std, err := stdout.NewExporter(stdout.Options{PrettyPrint: true})
	if err != nil {
		log.Fatal(err)
	}

	traceProvider, err := sdktrace.NewProvider(sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithSyncer(std))
	if err != nil {
		log.Fatal(err)
	}

	global.SetTraceProvider(traceProvider)

	mux := http.NewServeMux()
	mux.Handle("/", othttp.NewHandler(http.HandlerFunc(rootHandler), "root", othttp.WithPublicEndpoint()))
	mux.Handle("/calculate", othttp.NewHandler(http.HandlerFunc(calcHandler), "calculate", othttp.WithPublicEndpoint()))
	services = GetServices()

	log.Println("Initializing server...")
	err = http.ListenAndServe(":3000", mux)
	if err != nil {
		log.Fatalf("Could not initialize server: %s", err)
	}
}

func rootHandler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	trace.CurrentSpan(ctx).AddEvent(ctx, "called root handler, getting discovered services")
	fmt.Fprintf(w, "%s", services)
}

func calcHandler(w http.ResponseWriter, req *http.Request) {
	calcRequest, err := ParseCalcRequest(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var url string

	for _, n := range services.Services {
		if strings.ToLower(calcRequest.Method) == strings.ToLower(n.Name) {
			j, _ := json.Marshal(calcRequest.Operands)
			url = fmt.Sprintf("http://%s:%d/%s?o=%s", n.Host, n.Port, strings.ToLower(n.Name), strings.Trim(string(j), "[]"))
		} else {
			http.Error(w, "could not find requested calculation method", http.StatusBadRequest)
		}
	}

	client := http.DefaultClient
	ctx := req.Context()
	request, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	ctx, request = httptrace.W3C(ctx, request)
	httptrace.Inject(ctx, request)
	res, err := client.Do(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := strconv.Atoi(string(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "%d", resp)
}