package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	tm "github.com/buger/goterm"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
)

var wptDataPath *string
var projectId *string
var inputGcsBucket *string
var outputGcsBucket *string
var wptdHost *string

// TODO: This is copied from wptdashboard/models.go, but importing
// wptdashboard does not appear to be working.
type TestRun struct {
	// Platform information
	BrowserName    string `json:"browser_name"`
	BrowserVersion string `json:"browser_version"`
	OSName         string `json:"os_name"`
	OSVersion      string `json:"os_version"`

	// The first 10 characters of the SHA1 of the tested WPT revision
	Revision string `json:"revision"`

	// Results URL
	ResultsURL string `json:"results_url"`

	CreatedAt time.Time `json:"created_at"`
}

type TestRunSlice []TestRun

func (s TestRunSlice) Len() int {
	return len(s)
}

func (s TestRunSlice) Less(i int, j int) bool {
	if s[i].Revision < s[j].Revision {
		return true
	}
	if s[i].Revision > s[j].Revision {
		return false
	}
	if s[i].BrowserName < s[j].BrowserName {
		return true
	}
	if s[i].BrowserName > s[j].BrowserName {
		return false
	}
	if s[i].BrowserVersion < s[j].BrowserVersion {
		return true
	}
	if s[i].BrowserVersion > s[j].BrowserVersion {
		return false
	}
	if s[i].OSName < s[j].OSName {
		return true
	}
	if s[i].OSName > s[j].OSName {
		return false
	}
	return s[i].OSVersion < s[j].OSVersion
}

func (s TestRunSlice) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}

type SubTest struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Message *string `json:"message"`
}

type TestResults struct {
	Test     string    `json:"test"`
	Status   string    `json:"status"`
	Message  *string   `json:"message"`
	Subtests []SubTest `json:"subtests"`
}

type TestRunResults struct {
	Run *TestRun
	Res *TestResults
}

type TestId struct {
	Test string `json:"test"`
	Name string `json:"name"`
}

type TestIdSlice []TestId

func (s TestIdSlice) Len() int {
	return len(s)
}

func (s TestIdSlice) Less(i int, j int) bool {
	if s[i].Test < s[j].Test {
		return true
	}
	if s[i].Test > s[j].Test {
		return false
	}
	return s[i].Name < s[j].Name
}

func (s TestIdSlice) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}

// Copied from generated/test_status.pb.go.
type TestStatus int32

const (
	TestStatus_TEST_STATUS_UNKNOWN TestStatus = 0
	TestStatus_TEST_OK             TestStatus = 1
	TestStatus_TEST_ERROR          TestStatus = 2
	TestStatus_TEST_TIMEOUT        TestStatus = 3
)

var TestStatus_name = map[int32]string{
	0: "TEST_STATUS_UNKNOWN",
	1: "TEST_OK",
	2: "TEST_ERROR",
	3: "TEST_TIMEOUT",
}
var TestStatus_value = map[string]int32{
	"TEST_STATUS_UNKNOWN": 0,
	"TEST_OK":             1,
	"TEST_ERROR":          2,
	"TEST_TIMEOUT":        3,
}

func TestStatus_fromString(str string) (ts TestStatus) {
	value, ok := TestStatus_value["TEST_"+str]
	if !ok {
		return TestStatus_TEST_STATUS_UNKNOWN
	}
	return TestStatus(value)
}

type SubTestStatus int32

const (
	SubTestStatus_SUB_TEST_STATUS_UNKNOWN SubTestStatus = 0
	SubTestStatus_SUB_TEST_PASS           SubTestStatus = 1
	SubTestStatus_SUB_TEST_FAIL           SubTestStatus = 2
	SubTestStatus_SUB_TEST_TIMEOUT        SubTestStatus = 3
	SubTestStatus_SUB_TEST_NOT_RUN        SubTestStatus = 4
)

// Copied from generated/sub_test_status.pb.go.
var SubTestStatus_name = map[int32]string{
	0: "SUB_TEST_STATUS_UNKNOWN",
	1: "SUB_TEST_PASS",
	2: "SUB_TEST_FAIL",
	3: "SUB_TEST_TIMEOUT",
	4: "SUB_TEST_NOT_RUN",
}
var SubTestStatus_value = map[string]int32{
	"SUB_TEST_STATUS_UNKNOWN": 0,
	"SUB_TEST_PASS":           1,
	"SUB_TEST_FAIL":           2,
	"SUB_TEST_TIMEOUT":        3,
	"SUB_TEST_NOT_RUN":        4,
}

func SubTestStatus_fromString(str string) (ts SubTestStatus) {
	value, ok := SubTestStatus_value["SUB_TEST_"+str]
	if !ok {
		return SubTestStatus_SUB_TEST_STATUS_UNKNOWN
	}
	return SubTestStatus(value)
}

type CompleteTestStatus struct {
	Status    TestStatus
	SubStatus SubTestStatus
}

type TestRunStatus struct {
	Run    *TestRun
	Status CompleteTestStatus
}

type MetricsRun struct {
	StartTime *time.Time    `json:"start_time"`
	EndTime   *time.Time    `json:"end_time"`
	TestRuns  *TestRunSlice `json:"test_runs"`
}

type MetricsRunData struct {
	MetricsRun *MetricsRun `json:"metrics_run"`
	Data       interface{} `json:"data"`
}

// func gatherTestIds(allResults *[]TestRunResults) (allIds TestIdSlice) {
// 	allIdsMap := make(map[TestId]bool)
// 	for _, results := range *allResults {
// 		result := results.Res
// 		allIdsMap[TestId{Test: result.Test}] = true
// 		for _, subResult := range result.Subtests {
// 			allIdsMap[TestId{
// 				Test: result.Test,
// 				Name: subResult.Name,
// 			}] = true
// 		}
// 	}

// 	allIds = make(TestIdSlice, 0, len(allIdsMap))
// 	for testId := range allIdsMap {
// 		allIds = append(allIds, testId)
// 	}
// 	sort.Sort(allIds)

// 	return allIds
// }

type Path string

type Passes func(*CompleteTestStatus) bool

func uploadMetricsRunData(ctx context.Context, cs *storage.Client,
	bucket *storage.BucketHandle, objName *string,
	metricsRunData *MetricsRunData) (err error) {

	log.Println("Writing " + *objName + " to Google Cloud Storage")
	obj := bucket.Object(*objName)
	objWriter := obj.NewWriter(ctx)
	defer objWriter.Close()
	gzWriter := gzip.NewWriter(objWriter)
	defer gzWriter.Close()
	encoder := json.NewEncoder(gzWriter)
	err = encoder.Encode(metricsRunData)
	if err != nil {
		log.Printf("Error writing %s to Google Cloud Storage: %v\n",
			*objName, err)
		return err
	}
	log.Println("Wrote " + *objName + " to Google Cloud Storage")

	return err
}

func okAndUnknonwOrPasses(status *CompleteTestStatus) bool {
	return status.Status == TestStatus_TEST_OK &&
		(status.SubStatus == SubTestStatus_SUB_TEST_STATUS_UNKNOWN ||
			status.SubStatus == SubTestStatus_SUB_TEST_PASS)
}

func computeTotals(results *map[TestId]map[TestRun]CompleteTestStatus) (
	metrics map[Path]int) {
	metrics = make(map[Path]int)

	for testId := range *results {
		pathParts := strings.Split(testId.Test, "/")
		for i := range pathParts {
			subPath := Path(strings.Join(pathParts[:i+1],
				"/"))
			_, ok := metrics[subPath]
			if !ok {
				metrics[subPath] = 0
			}
			metrics[subPath] = metrics[subPath] + 1
		}
	}

	return metrics
}

func computeBrowserFailureList(
	numRuns int,
	browserName string,
	results *map[TestId]map[TestRun]CompleteTestStatus,
	passes Passes) (failures [][]*TestId) {
	failures = make([][]*TestId, numRuns)

	for testId, runStatuses := range *results {
		numOtherFailures := 0
		browserFailed := false
		for run, status := range runStatuses {
			if !passes(&status) {
				if run.BrowserName == browserName {
					browserFailed = true
				} else {
					numOtherFailures++
				}
			}
		}
		if !browserFailed {
			continue
		}
		failures[numOtherFailures] = append(failures[numOtherFailures],
			&testId)
	}

	return failures
}

func computePassRateMetric(numRuns int,
	results *map[TestId]map[TestRun]CompleteTestStatus, passes Passes) (
	metrics map[Path][]int) {
	metrics = make(map[Path][]int)

	for testId, runStatuses := range *results {
		passCount := 0
		for _, status := range runStatuses {
			if passes(&status) {
				passCount++
			}
		}
		pathParts := strings.Split(testId.Test, "/")
		for i := range pathParts {
			subPath := Path(strings.Join(pathParts[:i+1],
				"/"))
			_, ok := metrics[subPath]
			if !ok {
				metrics[subPath] = make([]int, numRuns+1)
			}
			metrics[subPath][passCount] =
				metrics[subPath][passCount] + 1
		}
	}

	return metrics
}

func gatherResultsById(allResults *[]TestRunResults) (
	resultsById map[TestId]map[TestRun]CompleteTestStatus) {
	resultsById = make(map[TestId]map[TestRun]CompleteTestStatus)

	for _, results := range *allResults {
		result := results.Res
		run := *results.Run
		testId := TestId{Test: result.Test}
		_, ok := resultsById[testId]
		if !ok {
			resultsById[testId] = make(
				map[TestRun]CompleteTestStatus)

		}
		_, ok = resultsById[testId][run]
		if ok {
			log.Printf("Duplicate results for TestId:%v  in "+
				"TestRun:%v.  Overwriting.\n", testId, run)
		}
		newStatus := CompleteTestStatus{
			Status: TestStatus_fromString(result.Status),
		}
		resultsById[testId][run] = newStatus

		for _, subResult := range result.Subtests {
			testId := TestId{
				Test: result.Test,
				Name: subResult.Name,
			}
			_, ok := resultsById[testId]
			if !ok {
				resultsById[testId] = make(
					map[TestRun]CompleteTestStatus)
			}
			_, ok = resultsById[testId][run]
			if ok {
				log.Printf("Duplicate sub-results for "+
					"TestId:%v  in TestRun:%v.  "+
					"Overwriting.\n", testId, run)
			}
			newStatus := CompleteTestStatus{
				Status: TestStatus_fromString(result.Status),
				SubStatus: SubTestStatus_fromString(
					subResult.Status),
			}
			resultsById[testId][run] = newStatus
		}
	}

	return resultsById
}

func loadTestResults(ctx context.Context, cs *storage.Client,
	bucket *storage.BucketHandle, testRun *TestRun, objName string,
	resultChan chan TestRunResults, errChan chan error) {
	// Read object from GCS
	obj := bucket.Object(objName)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		errChan <- err
		return
	}
	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		errChan <- err
		return
	}

	// Unmarshal JSON, which may be gzipped.
	var results TestResults
	var anyResult interface{}
	if err := json.Unmarshal(data, &anyResult); err != nil {
		reader2 := bytes.NewReader(data)
		reader3, err := gzip.NewReader(reader2)
		if err != nil {
			errChan <- err
			return
		}
		defer reader3.Close()
		unzippedData, err := ioutil.ReadAll(reader3)
		if err != nil {
			errChan <- err
			return
		}
		if err := json.Unmarshal(unzippedData, &results); err != nil {
			errChan <- err
			return
		}
		resultChan <- TestRunResults{testRun, &results}
	} else {
		if err := json.Unmarshal(data, &results); err != nil {
			errChan <- err
			return
		}
		resultChan <- TestRunResults{testRun, &results}
	}
}

func processTestRun(ctx context.Context, cs *storage.Client,
	bucket *storage.BucketHandle, testRun *TestRun,
	resultChan chan TestRunResults, errChan chan error) {
	resultsURL := testRun.ResultsURL

	// summaryURL format:
	//
	// protocol://host/bucket/dir/path-summary.json.gz
	//
	// where results are stored in
	//
	// protocol://host/bucket/dir/path/**
	//
	// Desired bucket-relative GCS prefix:
	//
	// dir/path/
	prefixSliceStart := strings.Index(resultsURL, *inputGcsBucket) +
		len(*inputGcsBucket) + 1
	prefixSliceEnd := strings.LastIndex(resultsURL, "-")
	prefix := resultsURL[prefixSliceStart:prefixSliceEnd] + "/"

	// Get objects with desired prefix, process them in parallel, then
	// return.
	it := bucket.Objects(ctx, &storage.Query{
		Prefix: prefix,
	})
	var wg sync.WaitGroup
	wg.Add(1)

	for {
		var err error
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			errChan <- err
			continue
		}

		// Skip directories.
		if attrs.Name == "" {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			loadTestResults(ctx, cs, bucket, testRun, attrs.Name,
				resultChan, errChan)
		}()
	}
	wg.Done()
	wg.Wait()
}

func loadTestRuns(ctx context.Context, cs *storage.Client,
	bucket *storage.BucketHandle,
	runs []TestRun) (runResults []TestRunResults) {
	resultChan := make(chan TestRunResults, 0)
	errChan := make(chan error, 0)
	runResults = make([]TestRunResults, 0, 100000)

	go func() {
		defer close(resultChan)
		defer close(errChan)

		var wg sync.WaitGroup
		wg.Add(len(runs))
		for _, run := range runs {
			go func(run TestRun) {
				defer wg.Done()
				processTestRun(ctx, cs, bucket, &run,
					resultChan, errChan)
			}(run)
		}
		wg.Wait()
	}()

	progress := make(map[TestRun]int)
	type Nothing struct{}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for results := range resultChan {
			runResults = append(runResults, results)

			testRunPtr := results.Run
			testRun := *testRunPtr
			if _, ok := progress[testRun]; !ok {
				progress[testRun] = 0
			}
			progress[testRun] = progress[testRun] + 1

			keys := make(TestRunSlice, 0, len(progress))
			for key := range progress {
				keys = append(keys, key)
			}
			sort.Sort(keys)

			tm.Clear()
			tm.MoveCursor(1, 1)
			for _, run := range keys {
				count := progress[run]
				tm.Printf("%10s %10s %10s %10s %10s :: %10d\n",
					run.Revision, run.BrowserName,
					run.BrowserVersion, run.OSName,
					run.OSVersion, count)
			}
			tm.Flush()
		}
	}()
	go func() {
		defer wg.Done()
		for err := range errChan {
			log.Fatal(err)
		}
	}()
	wg.Wait()

	return runResults
}

func getRuns() TestRunSlice {
	url := "https://" + *wptdHost + "/api/runs"
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != 200 {
		log.Fatal(errors.New("Bad response code from " + url + ": " +
			strconv.Itoa(resp.StatusCode)))
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var runs TestRunSlice
	if err := json.Unmarshal(body, &runs); err != nil {
		log.Fatal(err)
	}
	return runs
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	logFileName := "current_metrics.log"
	logFile, err := os.OpenFile(logFileName, os.O_RDWR|os.O_CREATE|
		os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	defer logFile.Close()
	log.Printf("Logs appended to %s\n", logFileName)
	log.SetOutput(logFile)

	wptDataPath = flag.String("wpt_data_path", os.Getenv("HOME")+
		"/wpt-data", "Path to data directory for local data copied "+
		"from Google Cloud Storage")
	projectId = flag.String("project_id", "wptdashboard",
		"Google Cloud Platform project id")
	inputGcsBucket = flag.String("input_gcs_bucket", "wptd",
		"Google Cloud Storage bucket where test results are stored")
	outputGcsBucket = flag.String("output_gcs_bucket", "wptd-metrics",
		"Google Cloud Storage bucket where metrics are stored")
	wptdHost = flag.String("wptd_host", "wpt.fyi",
		"Hostname of endpoint that serves WPT Dashboard data API")

	ctx := context.Background()
	cs, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	inputBucket := cs.Bucket(*inputGcsBucket)

	log.Println("Reading test results from Google Cloud Storage")

	readStartTime := time.Now()
	runs := getRuns()
	allResults := loadTestRuns(ctx, cs, inputBucket, runs)
	readEndTime := time.Now()

	log.Println("Read test results from Google Cloud Storage")
	log.Println("Consolidating results")

	resultsById := gatherResultsById(&allResults)

	log.Println("Consolidated results")
	log.Println("Computing metrics")

	var totals map[Path]int
	var passRateMetric map[Path][]int
	failuresMetrics := make(map[string][][]*TestId)
	var wg sync.WaitGroup
	wg.Add(2 + len(runs))
	go func() {
		defer wg.Done()
		totals = computeTotals(&resultsById)
	}()
	go func() {
		defer wg.Done()
		passRateMetric = computePassRateMetric(len(runs), &resultsById,
			okAndUnknonwOrPasses)
	}()
	for _, run := range runs {
		go func(browserName string) {
			defer wg.Done()
			// TODO: Check that browser names are different
			failuresMetrics[browserName] =
				computeBrowserFailureList(len(runs),
					browserName, &resultsById,
					okAndUnknonwOrPasses)
		}(run.BrowserName)
	}
	wg.Wait()

	log.Println("Computed metrics")
	log.Println("Writing metrics to Google Cloud Storage")

	outputBucket := cs.Bucket(*outputGcsBucket)
	metricsRun := MetricsRun{
		StartTime: &readStartTime,
		EndTime:   &readEndTime,
		TestRuns:  &runs,
	}

	wg.Add(2 + len(failuresMetrics))
	go func() {
		defer wg.Done()
		objName := fmt.Sprintf("%d-%d/pass-rates.json.gz",
			metricsRun.StartTime.Unix(),
			metricsRun.EndTime.Unix())
		passRateSummary := MetricsRunData{
			MetricsRun: &metricsRun,
			Data:       &passRateMetric,
		}
		err := uploadMetricsRunData(ctx, cs, outputBucket, &objName,
			&passRateSummary)
		if err != nil {
			log.Println(err)
		}
	}()
	go func() {
		defer wg.Done()
		objName := fmt.Sprintf("%d-%d/test-counts.json.gz",
			metricsRun.StartTime.Unix(),
			metricsRun.EndTime.Unix())
		totalsSummary := MetricsRunData{
			MetricsRun: &metricsRun,
			Data:       &totals,
		}
		err := uploadMetricsRunData(ctx, cs, outputBucket, &objName,
			&totalsSummary)
		if err != nil {
			log.Println(err)
		}
	}()
	for browserName, metrics := range failuresMetrics {
		go func(browserName string, metrics [][]*TestId) {
			defer wg.Done()
			objName := fmt.Sprintf("%d-%d/failures-%s.json.gz",
				metricsRun.StartTime.Unix(),
				metricsRun.EndTime.Unix(),
				browserName)
			failureSummary := MetricsRunData{
				MetricsRun: &metricsRun,
				Data:       &metrics,
			}
			err := uploadMetricsRunData(ctx, cs, outputBucket,
				&objName, &failureSummary)
			if err != nil {
				log.Println(err)
			}
		}(browserName, metrics)
	}
	wg.Wait()

	log.Println("Wrote metrics to Google Cloud Storage")

	// i := 0
	// for path, metricSlice := range passRateMetric {
	// 	i++
	// 	log.Println(path)
	// 	log.Println(metricSlice)
	// 	log.Println(totals[path])
	// 	if i >= 10 {
	// 		break
	// 	}
	// }
	// for browserName, metrics := range failuresMetrics {
	// 	counts := make([]int, len(metrics))
	// 	for i, values := range metrics {
	// 		counts[i] = len(values)
	// 	}
	// 	log.Printf("%s %v", browserName, counts)
	// }

	// countMap := make(map[int]int)
	// for _, runStatusMap := range resultsById {
	// 	count := len(runStatusMap)
	// 	current, ok := countMap[count]
	// 	if !ok {
	// 		current = 0
	// 	}
	// 	countMap[count] = current + 1

	// 	// if count > 4 {
	// 	// 	log.Println("-----------------------------------------")
	// 	// 	log.Println(testId.Test)
	// 	// 	log.Println(testId.Name)
	// 	// 	for _, runStatuses := range *runStatusSlicePtr {
	// 	// 		log.Println(runStatuses.Run)
	// 	// 		log.Println(runStatuses.Status)
	// 	// 	}
	// 	// }
	// }
	// for count, countOfCount := range countMap {
	// 	log.Printf("%d  %d", count, countOfCount)
	// }
	// // for testId, runStatusSlicePtr := range resultsById {
	// // 	count++
	// // 	log.Println("-----------------------------------------")
	// // 	log.Println(testId)
	// // 	for _, runStatuses := range *runStatusSlicePtr {
	// // 		log.Println(runStatuses.Run)
	// // 		log.Println(runStatuses.Status)
	// // 	}
	// // 	if count >= 10 {
	// // 		break
	// // 	}
	// // }
	// log.Println(len(resultsById))
}
