package main

import (
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
)

// ProductTypeEndpointValue represents a service availability metric for a specific product, type, and endpoint.
type ProductTypeEndpointValue struct {
	Product  string
	Type     string
	Endpoint string
	Value    float64
}

// CollectPromMetrics collects Prometheus metrics and sends them to the provided channel.
func (e *Exporter) CollectPromMetrics(ch chan<- prometheus.Metric) {
	err := e.TestProm()
	if err != nil {
		ch <- prometheus.MustNewConstMetric(
			up, prometheus.GaugeValue, 0, "prometheus",
		)
		log.Error(err)
		return
	}
	ch <- prometheus.MustNewConstMetric(
		up, prometheus.GaugeValue, 1, "prometheus",
	)

	e.HitProm(ch)
}

// TestProm tests connectivity to Prometheus server.
func (e *Exporter) TestProm() error {
	_, err := PromQuery(e.promURL, "up{job=\"prometheus\"}")

	if err != nil {
		log.Error("Prometheus dependancy NOK")
	} else {
		log.Info("Prometheus dependancy OK")
	}
	return err
}

// HitProm queries Prometheus for service availability metrics and publishes them.
// https://godoc.org/github.com/prometheus/common/model#Vector
// https://godoc.org/github.com/prometheus/client_golang/api/prometheus/v1
func (e *Exporter) HitProm(ch chan<- prometheus.Metric) {

	//sa_internal interactive
	saInternalInteractive := e.GetMetricSaInternal("interactive", e.saInteractiveAggr)
	for _, elem := range saInternalInteractive {
		ch <- prometheus.MustNewConstMetric(
			metricSaInternal, prometheus.GaugeValue, elem.Value, elem.Product, elem.Type, elem.Endpoint,
		)
	}

	//sa_internal batch
	saInternalBatch := e.GetMetricSaInternal("batch", e.saBatchAggr)
	for _, elem := range saInternalBatch {
		ch <- prometheus.MustNewConstMetric(
			metricSaInternal, prometheus.GaugeValue, elem.Value, elem.Product, elem.Type, elem.Endpoint,
		)
	}

	//BY PRODUCT Metrics
	//find all unique products from batch list
	products := FindProductsFromQueryResult(saInternalBatch)
	for product := range products {
		log.Info("Will compute SA aggr metrics for product : ", product)
		//sa_interactive
		saInteractiveLabel := "interactive"
		saInteractive := ZeroAlwaysWin(ExtractValues(product, saInternalInteractive), product+" "+saInteractiveLabel)
		ch <- prometheus.MustNewConstMetric(
			metricSaType, prometheus.GaugeValue, saInteractive, product, saInteractiveLabel,
		)
		//sa_batch
		saBatchLabel := "batch"
		saBatch := ZeroAlwaysWin(ExtractValues(product, saInternalBatch), product+" "+saBatchLabel)
		ch <- prometheus.MustNewConstMetric(
			metricSaType, prometheus.GaugeValue, saBatch, product, saBatchLabel,
		)
		//sa_overall
		saOverall := ZeroAlwaysWin([]float64{saInteractive, saBatch}, product+" overall")
		ch <- prometheus.MustNewConstMetric(
			metricSaOverall, prometheus.GaugeValue, saOverall, product,
		)
	}

	log.Debug("Endpoint scraped")
}

// GetMetricSaInternal retrieves service availability metrics for internal endpoints of a specific type.
func (e *Exporter) GetMetricSaInternal(typeEndpoint string, aggr string) []ProductTypeEndpointValue {
	//Build the PromQL query returning all interactive|batch endpoints ready values
	//Q : min by(endpoint) (min_over_time(kube_endpoint_address_not_ready{namespace="test"}[5m]))
	//R : {endpoint="test-svc"}
	//change the behavior to  kube_endpoint_address_available (should use max instead of min)
	//NEW Feb 2025
	//kube_endpoint_address_available was deprecated in 2.5.0 then removed in 2.14.0
	//kube_endpoint_address is the new metric to use
	var result []ProductTypeEndpointValue
	queryEndpoints := BuildSaQueryEndpoints(typeEndpoint, e.mapKeyType)

	//1. find the total number of addresses ready or not
	mapEndpointAvail := make(map[string]float64)
	queryAllAdressSvc := "sum by (endpoint)(kube_endpoint_address{endpoint=~\"" + queryEndpoints + "\"})"
	dataAllAdressSvc, err := PromQuery(e.promURL, queryAllAdressSvc)
	if err != nil {
		log.Error("PromQL query wrong for ", queryAllAdressSvc)
	} else {
		log.Info("GetMetricSaInternal query : ", queryAllAdressSvc)
		vectorVal := dataAllAdressSvc.(model.Vector)
		for _, elem := range vectorVal {
			endpoint := elem.Metric["endpoint"]
			mapEndpointAvail[string(endpoint)] = float64(elem.Value)
		}
	}

	//2. find the total number of addresses not ready and substract it from the total
	queryNotReadyAddressSvc := "sum by (endpoint)(kube_endpoint_address{endpoint=~\"" + queryEndpoints + "\",ready=\"false\"})"
	dataNotReadyAddressSvc, err := PromQuery(e.promURL, queryNotReadyAddressSvc)
	if err != nil {
		log.Error("PromQL query wrong for ", queryNotReadyAddressSvc)
	} else {
		vectorVal := dataNotReadyAddressSvc.(model.Vector)
		for _, elem := range vectorVal {
			endpoint := elem.Metric["endpoint"]
			if _, ok := mapEndpointAvail[string(endpoint)]; !ok {
				log.Error("Endpoint not found in mapEndpointAvail, synch issue !: ", string(endpoint))
				continue
			}
			mapEndpointAvail[string(endpoint)] = mapEndpointAvail[string(endpoint)] - float64(elem.Value)
		}
	}

	//3. find the total number of addresses available (ie total- not ready)
	//if total == 0 => SA == 0
	//if total > 0 => SA == 1
	for endpoint, value := range mapEndpointAvail {
		readyValue := ReadyValue(value)
		if readyValue < 1.0 {
			log.Info("SA DOWN for endpoint : ", endpoint, " , #address_available : ", value)
		}

		// it is possible to have multiple products
		products := FindProductsForEndpoint(endpoint, e.mapKeyEndpoint)
		for _, product := range products {
			result = append(result, ProductTypeEndpointValue{product, typeEndpoint, endpoint, readyValue})
		}
	}

	return result
}

// BuildSaQueryEndpoints builds a pipe-separated string of endpoints for the given type.
func BuildSaQueryEndpoints(typeEndpoint string, mapKeyType map[string][]string) string {
	var result strings.Builder
	endpoints := mapKeyType[typeEndpoint]
	for i := 0; i < len(endpoints); i++ {
		result.WriteString(endpoints[i])
		result.WriteString("|")
	}
	return result.String()
}

// FindProductsForEndpoint finds all products associated with a given endpoint using regex matching.
func FindProductsForEndpoint(endpointToTest string, mapKeyEndpoint map[string][]string) []string {
	//a map here is of no help unfortunately
	var result []string
	for endpoint, products := range mapKeyEndpoint {
		pattern := regexp.MustCompile(endpoint)
		if pattern.MatchString(endpointToTest) {
			log.Debug(endpointToTest, " is matching with ", endpoint)
			return products
		}
	}

	if len(result) == 0 {
		log.Error("Could not found any matches for endpoint " + endpointToTest)
	}

	return result
}

// FindProductsFromQueryResult extracts unique product names from query results.
func FindProductsFromQueryResult(productTypeEndpointValue []ProductTypeEndpointValue) map[string]struct{} {
	var member struct{}
	set := make(map[string]struct{})
	for _, elem := range productTypeEndpointValue {
		set[elem.Product] = member
	}
	return set
}

// ReadyValue converts a numeric value to a binary ready state (0 or 1).
func ReadyValue(valueIn float64) float64 {
	valueOut := 1.0
	// if 0 => ready == 0, if > 0 => ready == 1
	if valueIn == 0.0 {
		valueOut = 0.0
	}
	return valueOut
}

// ExtractValues extracts all metric values for a specific product from the query results.
func ExtractValues(product string, productTypeEndpointValue []ProductTypeEndpointValue) []float64 {
	var result []float64
	for _, elem := range productTypeEndpointValue {
		if elem.Product == product {
			result = append(result, elem.Value)
		}
	}
	return result
}

// ZeroAlwaysWin implements the "zero always wins" logic for service availability.
// If any value is less than 1.0, the result is 0.0, otherwise 1.0.
func ZeroAlwaysWin(valuesIn []float64, kind string) float64 {
	// if at least one 0 => SA == 0
	for _, valueIn := range valuesIn {
		if valueIn < 1.0 {
			log.Info("SA DOWN for ", kind)
			return 0.0
		}
	}
	return 1.0
}
