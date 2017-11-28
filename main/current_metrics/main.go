package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
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
	"google.golang.org/api/option"
)

var wptDataPath *string
var projectId *string
var gcsBucket *string
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
	Test string  `json:"test"`
	Name *string `json:"name"`
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
	if s[i].Name == nil && s[j].Name != nil {
		return true
	}
	if s[i].Name != nil && s[j].Name == nil {
		return false
	}
	return *s[i].Name < *s[j].Name
}

func (s TestIdSlice) Swap(i int, j int) {
	s[i], s[j] = s[j], s[i]
}

func gatherTestIds(allResults *[]TestRunResults) (allIds TestIdSlice) {
	allIdsMap := make(map[TestId]bool)
	for _, results := range *allResults {
		result := results.Res
		allIdsMap[TestId{Test: result.Test}] = true
		for _, subResult := range result.Subtests {
			allIdsMap[TestId{
				Test: result.Test,
				Name: &subResult.Name,
			}] = true
		}
	}

	allIds = make(TestIdSlice, 0, len(allIdsMap))
	for testId := range allIdsMap {
		allIds = append(allIds, testId)
	}
	sort.Sort(allIds)

	return allIds
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
	prefixSliceStart := strings.Index(resultsURL, *gcsBucket) +
		len(*gcsBucket) + 1
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
	tm.Clear()

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

			tm.MoveCursor(1, 1)
			for _, run := range keys {
				count := progress[run]
				tm.Printf("%s %s %s %s %s :: %d\n",
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
	wptDataPath = flag.String("wpt_data_path", os.Getenv("HOME")+"/wpt-data", "Path to data directory for local data copied from Google Cloud Storage")
	projectId = flag.String("project_id", "wptdashboard", "Google Cloud Platform project id")
	gcsBucket = flag.String("gcs_bucket", "wptd", "Google Cloud Storage bucket where test results are stored")
	wptdHost = flag.String("wptd_host", "wpt.fyi", "Hostname of endpoint that serves WPT Dashboard data API")

	ctx := context.Background()
	cs, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		log.Fatal(err)
	}
	bucket := cs.Bucket("wptd")

	allResults := loadTestRuns(ctx, cs, bucket, getRuns())
	allIds := gatherTestIds(&allResults)

	log.Println(len(allIds))
}
