package main

import (
	// "cloud.google.com/go/bigquery"
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	models "wptdashboard"
	protos "wptdashboard/generated"

	"cloud.google.com/go/datastore"
	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/ptypes"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type Cmd struct {
	dir    *string
	args   []string
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (cmd Cmd) Id() string {
	var id = "["
	if cmd.dir != nil {
		id += *cmd.dir + ":"
	}
	id += strings.Join(cmd.args, " ") + "]"
	return id
}

func (cmd Cmd) Start() (err error) {
	log.Printf("Starting %s\n", cmd.Id())
	err = cmd.cmd.Start()
	if err != nil {
		log.Printf("Started %s (error=%s)\n", cmd.Id(), err)
	} else {
		log.Printf("Started %s\n", cmd.Id())
	}
	return err
}

func (cmd Cmd) Wait() (err error) {
	log.Printf("Draining output from %s\n", cmd.Id())
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ioutil.ReadAll(cmd.stdout)
	}()
	go func() {
		defer wg.Done()
		ioutil.ReadAll(cmd.stderr)
	}()
	wg.Wait()

	log.Printf("Waiting for to complete %s\n", cmd.Id())
	err = cmd.cmd.Wait()
	if err != nil {
		log.Printf("Completed for %s with (error=%s)\n", cmd.Id(), err)
	} else {
		log.Printf("Completed %s\n", cmd.Id())
	}
	return err
}

func setupLogger(file *os.File, prefix string, flags int, reader io.ReadCloser) io.ReadCloser {
	logger := log.New(file, prefix, flags)
	pipeReader, pipeWriter := io.Pipe()
	teeReader := io.TeeReader(reader, pipeWriter)
	scanner := bufio.NewScanner(teeReader)

	go func() {
		defer pipeWriter.Close()
		for scanner.Scan() {
			logger.Println(scanner.Text())
		}
		err := scanner.Err()
		if err != nil && err != io.EOF {
			log.Printf("Error forwarding scanner to logger: %s %s\n", prefix, err)
		}
	}()

	return pipeReader
}

func makeCmd(dir *string, args ...string) (ret Cmd) {
	cmd := exec.Command(args[0], args[1:]...)
	if dir != nil {
		cmd.Dir = *dir
	}
	ret = Cmd{
		dir,
		args,
		cmd,
		nil,
		nil,
	}

	var err error
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	cmdId := ret.Id()
	ret.stdout = setupLogger(os.Stdout, cmdId, log.LstdFlags, stdout)
	ret.stderr = setupLogger(os.Stderr, cmdId, log.LstdFlags, stderr)
	return ret
}

type ChanCmd struct {
	cmd        Cmd
	stdoutChan chan string
	stderrChan chan string
}

func makeChanCmd(dir *string, args ...string) ChanCmd {
	return ChanCmd{makeCmd(dir, args...), make(chan string), make(chan string)}
}

func (chanCmd ChanCmd) Start(errChan chan error) {
	cmd := chanCmd.cmd
	if err := cmd.Start(); err != nil {
		errChan <- err
		close(errChan)
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	scan := func(reader io.ReadCloser, channel chan string) {
		defer close(channel)
		defer wg.Done()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			channel <- strings.TrimSpace(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			errChan <- err
		}
	}
	go scan(cmd.stdout, chanCmd.stdoutChan)
	go scan(cmd.stderr, chanCmd.stderrChan)
	go func() {
		defer close(errChan)
		wg.Wait()
	}()
}

func (chanCmd ChanCmd) Wait(errChan chan error) {
	cmd := chanCmd.cmd
	err := cmd.Wait()
	if err != nil {
		errChan <- err
	}
}

// func gsutilLsToChan(outChan chan string, errChan chan error, args ...string) {
// 	defer close(outChan)
// 	defer close(errChan)
// 	cmd := makeCmd(nil, append(append(append(make([]string, 0), "gsutil"), "ls"), args...)...)
// 	if err := cmd.Start(); err != nil {
// 		errChan <- err
// 		return
// 	}
// 	scanner := bufio.NewScanner(cmd.stdout)
// 	for scanner.Scan() {
// 		outChan <- strings.TrimSpace(scanner.Text())
// 	}
// 	if err := scanner.Err(); err != nil {
// 		errChan <- err
// 	}
// }

// func gsutilLs(args ...string) (entries []string, err error) {
// 	outChan := make(chan string)
// 	errChan := make(chan error)
// 	go gsutilLsToChan(outChan, errChan, args...)
// 	for err := range errChan {
// 		return entries, err
// 	}
// 	for entry := range outChan {
// 		entries = append(entries, entry)
// 	}
// 	return entries, err
// }

func filterGsUrlsToHashes(urls []string) (hashes []string) {
	hashes = make([]string, 0)
	for _, url := range urls {
		parts := strings.Split(url, "/")
		if len(parts) < 2 {
			continue
		}
		maybeHash := strings.TrimSpace(parts[len(parts)-2])
		matched, err := regexp.MatchString("^[0-9a-f]+$", maybeHash)
		if err != nil {
			continue
		}
		if matched {
			hashes = append(hashes, maybeHash)
		}
	}
	return hashes
}

type Commit struct {
	shortHash  string
	longHash   string
	commitTime time.Time
}

type ByCommitTime []*Commit

func (c ByCommitTime) Len() int {
	return len(c)
}

func (c ByCommitTime) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c ByCommitTime) Less(i, j int) bool {
	return c[i].commitTime.Before(c[j].commitTime)
}

func dropNilCommits(data []*Commit) []*Commit {
	ret := make([]*Commit, 0)
	for _, ptr := range data {
		if ptr != nil {
			ret = append(ret, ptr)
		}
	}
	return ret
}

func shortHashToLongHash(wptPath string, shortHash string) *string {
	cmd := makeCmd(&wptPath, "git", "log", "-1", "--format=%H", shortHash)
	if err := cmd.Start(); err != nil {
		log.Println(err)
		return nil
	}
	bytes, err := ioutil.ReadAll(cmd.stdout)
	go cmd.Wait()
	if err != nil {
		log.Println(err)
		return nil
	}
	str := strings.TrimSpace(string(bytes))
	return &str
}

func shortHashToTime(wptPath string, shortHash string) *time.Time {
	cmd := makeCmd(&wptPath, "git", "log", "-1", "--date=unix", "--format=%cd", shortHash)
	if err := cmd.Start(); err != nil {
		log.Println(err)
		return nil
	}
	bytes, err := ioutil.ReadAll(cmd.stdout)
	go cmd.Wait()
	if err != nil {
		log.Println(err)
		return nil
	}
	str := strings.TrimSpace(string(bytes))
	timestamp, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		log.Println(err)
		return nil
	}
	timeValue := time.Unix(timestamp, 0)
	return &timeValue
}

type CommitCacheKey struct {
	wptPath   string
	shortHash string
}

var commitCache map[CommitCacheKey]*Commit

func shortHashToCommit(wptPath string, shortHash string) (commit *Commit) {
	if commitCache == nil {
		commitCache = make(map[CommitCacheKey]*Commit)
	}
	commitCacheKey := CommitCacheKey{wptPath, shortHash}
	if commitCache[commitCacheKey] != nil {
		return commitCache[commitCacheKey]
	}

	longHashChan := make(chan *string)
	timeChan := make(chan *time.Time)
	go func() {
		longHashChan <- shortHashToLongHash(wptPath, shortHash)
	}()
	go func() {
		timeChan <- shortHashToTime(wptPath, shortHash)
	}()

	longHash := <-longHashChan
	commitTime := <-timeChan
	if longHash == nil || commitTime == nil {
		return nil
	}
	return &Commit{shortHash, *longHash, *commitTime}
}

func hashesToCommits(wptPath string, hashes []string) (commits []*Commit) {
	var wg sync.WaitGroup
	wg.Add(len(hashes))
	for _, hash := range hashes {
		go func(shortHash string) {
			defer wg.Done()
			commits = append(commits, shortHashToCommit(wptPath, shortHash))
		}(hash)
	}
	wg.Wait()

	return commits
}

// func gsutilCp(remoteBaseUrl string, localBasePath string, url string) error {
// 	sep := "/"
// 	if remoteBaseUrl[len(remoteBaseUrl)-1] == '/' {
// 		sep = ""
// 	}

// 	localPath := localBasePath + sep + url[len(localBasePath):]

// 	// Skip cp when file exists, or some weird error in the file system
// 	// occurred
// 	if _, err := os.Stat(localPath); err == nil || !os.IsNotExist(err) {
// 		return err
// 	}

// 	cmd := makeCmd(nil, "gsutil", "cp", url, localPath)
// 	if err := cmd.Start(); err != nil {
// 		return err
// 	}
// 	if err := cmd.Wait(); err != nil {
// 		return err
// 	}
// 	return nil
// }

// func gsutilCatRecursive(url string, path string) (err error) {
// 	lsOutChan := make(chan string)
// 	lsErrChan := make(chan error)
// 	cpErrChan := make(chan error)
// 	go gsutilLsToChan(lsOutChan, lsErrChan, "-r", url)
// 	go func() {
// 		defer close(cpErrChan)
// 		for entry := range lsOutChan {
// 			// Skip short lines and directories
// 			if len(entry) <= len(url) || entry[len(entry)-1] == '/' {
// 				continue
// 			}
// 			cpErr := gsutilCat(url, path, entry)
// 			if cpErr != nil {
// 				cpErrChan <- cpErr
// 			}
// 		}
// 	}()
// 	for lsErr := range lsErrChan {
// 		if err == nil {
// 			err = lsErr
// 		}
// 	}
// 	for cpErr := range cpErrChan {
// 		if err == nil {
// 			err = cpErr
// 		}
// 	}
// 	return err
// }

type SubTest struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message *string `json:"message"`
}

type TestResults struct {
	Test     string    `json:"test"`
	Status   string    `json:"status"`
	Message  *string   `json:"message"`
	Subtests []SubTest `json:"subtests"`
}

func unmarshalTestResult(data []byte) (testResults []protos.TestResult, err error) {
	var jsonTestResults TestResults
	err = json.Unmarshal(data, &jsonTestResults)
	if err != nil {
		return testResults, err
	}
	// TODO: Copy jsonTestResults into testResults
	return testResults, err
}

func hashesFromDatastore(ctx context.Context, client datastore.Client) (hashes []string, err error) {
	query := datastore.NewQuery("TestRun").Project("Revision")
	it := client.Run(ctx, query)

	// Dedup hashes with map
	seen := make(map[string]bool)
	for {
		var testRun models.TestRun
		_, err := it.Next(&testRun)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return hashes, err
		}
		if seen[testRun.Revision] {
			continue
		}
		seen[testRun.Revision] = true
		hashes = append(hashes, testRun.Revision)
	}
	return hashes, err
}

func createdAtFromShortHashDatastore(ctx context.Context, client datastore.Client, shortHash string) (createdAt time.Time, err error) {
	var testRuns []models.TestRun
	query := datastore.NewQuery("TestRun").Filter("Revision=", shortHash).Project("CreatedAt").Limit(1)
	client.GetAll(ctx, query, &testRuns)
	if len(testRuns) != 1 {
		return createdAt, errors.New("Failed to find revision in Datastore: " + shortHash)
	}
	return testRuns[0].CreatedAt, err
}

func hashesFromDataPath(dataPath string) (hashes []string, err error) {
	entries, err := ioutil.ReadDir(dataPath)
	if err != nil {
		return hashes, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			maybeHash := entry.Name()
			matched, matchErr := regexp.MatchString("^[0-9a-f]+$", maybeHash)
			if matchErr != nil {
				continue
			}
			if matched {
				hashes = append(hashes, maybeHash)
			}
		}
	}
	return hashes, err
}

func addPlatformToTestRun(platformStr string, testRun *protos.TestRun) (err error) {
	parts := strings.Split(platformStr, "-")
	if len(parts) > 0 {
		browserName := strings.ToUpper(parts[0])
		testRun.Browser = protos.Browser(protos.Browser_value[browserName])
	}
	if len(parts) > 1 {
		testRun.BrowserVersionStr = parts[1]
	}
	if len(parts) > 2 {
		osName := strings.ToUpper(parts[2])
		testRun.Os = protos.OperatingSystem(protos.OperatingSystem_value[osName])
	}
	if len(parts) > 3 {
		testRun.OsVersionStr = parts[3]
	}
	if len(parts) > 4 {
		return errors.New("Malformed platform string")
	}
	return nil
}

func addCommitToTestRun(commit Commit, testRun *protos.TestRun) (err error) {
	testRun.WptHash = commit.longHash
	protoCommitTime, err := ptypes.TimestampProto(commit.commitTime)
	if err == nil {
		testRun.WptCommitTime = protoCommitTime
	}
	return err
}

type TestRun struct {
	platformStr string
	testRun protos.TestRun
}

func testRunsFromDataPath(dataPath string, hash string) (testRuns []TestRun, err error) {
	entries, err := ioutil.ReadDir(dataPath + "/" + hash)
	if err != nil {
		return testRuns, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			platformStr := entry.Name()
			var testRun protos.TestRun
			if err := addPlatformToTestRun(platformStr, &testRun); err != nil {
				return testRuns, err
			}
			testRuns = append(testRuns, TestRun{platformStr, testRun})
		}
	}
	return testRuns, err
}

func getCommitsRemote(wptPath *string, ctx context.Context, ds *datastore.Client, cs *storage.Client, bucket *storage.BucketHandle) ([]Commit) {
	//
	// Wait for both commitsDS and commitsCS
	//
	var commitsDS []*Commit
	var commitsCS []*Commit
	var wg sync.WaitGroup
	wg.Add(2)

	//
	// Get commitsDS from Datastore
	//
	go func() {
		defer wg.Done()
		hashesDS, err := hashesFromDatastore(ctx, *ds)
		if err != nil {
			log.Fatal(err)
		}
		commitsDS = dropNilCommits(hashesToCommits(*wptPath, hashesDS))
		sort.Sort(sort.Reverse(ByCommitTime(commitsDS)))
	}()

	//
	// Get commitsCS from Cloud Storage
	//
	go func() {
		defer wg.Done()
		it := bucket.Objects(ctx, &storage.Query{Delimiter: "/"})
		hashes := make([]string, 0)
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Fatal(err)
			}
			// Prefix only set on directories, which is what we
			// seek.
			if attrs.Prefix != "" {
				// Drop trailing slash
				maybeHash := attrs.Prefix[:len(attrs.Prefix)-1]
				// Match as hash
				matched, matchErr := regexp.MatchString("^[0-9a-f]+$", maybeHash)
				if matchErr != nil {
					continue
				}
				if matched {
					hashes = append(hashes, maybeHash)
				}
			}
		}

		commitsCS = dropNilCommits(hashesToCommits(*wptPath, hashes))
		sort.Sort(sort.Reverse(ByCommitTime(commitsCS)))
	}()

	wg.Wait()
	goodRuns := make([]Commit, 0)
	for i, j := 0, 0; i < len(commitsDS) && j < len(commitsCS); {
		cmp := strings.Compare(commitsDS[i].shortHash, commitsCS[j].shortHash)
		if cmp < 0 {
			log.Printf("Lone DS: %s", commitsDS[i])
			i++
		} else if cmp > 0 {
			log.Printf("Lone CS: %s", commitsCS[j])
			j++
		} else {
			goodRuns = append(goodRuns, *commitsDS[i])
			i++
			j++
		}
	}

	return goodRuns
}

func catAndDecodeObjectRemote(ctx context.Context, cs *storage.Client, bucket *storage.BucketHandle, testRun protos.TestRun, objName string, resultChan chan protos.TestResult, errChan chan error) {
	obj := bucket.Object(objName)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		errChan <- err
		return
	}
	bytes, err := ioutil.ReadAll(reader)
	if err != nil {
		errChan <- err
		return
	}
	var results TestResults
	if err := json.Unmarshal(bytes, results); err != nil {
		errChan <- err
		return
	}

	statusName := strings.ToUpper(results.Status)
	status := protos.TestStatus(protos.TestStatus_value["TEST_" + statusName])
	var message string
	if results.Message == nil {
		message = ""
	} else {
		message = *results.Message
	}
	resultChan <- protos.TestResult {
		Os: testRun.Os,
		OsVersionStr: testRun.OsVersionStr,
		Browser: testRun.Browser,
		BrowserVersionStr: testRun.BrowserVersionStr,
		WptHash: testRun.WptHash,
		WptCommitTime: testRun.WptCommitTime,
		TestName: results.Test,
		Status: status,
		TestMessage: message,
	}
	for _, subTest := range results.Subtests {
		subStatusName := strings.ToUpper(subTest.Status)
		subStatus := protos.SubTestStatus(protos.SubTestStatus_value["SUB_TEST_" + subStatusName])
		var subMessage string
		if subTest.Message == nil {
			subMessage = ""
		} else {
			subMessage = *subTest.Message
		}
		resultChan <- protos.TestResult {
			Os: testRun.Os,
			OsVersionStr: testRun.OsVersionStr,
			Browser: testRun.Browser,
			BrowserVersionStr: testRun.BrowserVersionStr,
			WptHash: testRun.WptHash,
			WptCommitTime: testRun.WptCommitTime,
			TestName: results.Test,
			Status: status,
			TestMessage: message,
			TestSubName: subTest.Name,
			TestSubStatus: subStatus,
			TestSubMessage: subMessage,
		}
	}
}

func processTestRunResultsRemote(ctx context.Context, cs *storage.Client, bucket *storage.BucketHandle, shortHash string, platformStr string, testRun protos.TestRun, resultChan chan protos.TestResult, errChan chan error) {
	log.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!" + shortHash + platformStr)
	it := bucket.Objects(ctx, &storage.Query{
		Prefix: shortHash + "/" + platformStr + "/",
	})
	var wg sync.WaitGroup
	wg.Add(1)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			errChan <- err
			continue
		}
		if attrs.Name == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			catAndDecodeObjectRemote(ctx, cs, bucket, testRun, attrs.Name, resultChan, errChan)
		}()
	}
	wg.Done()
	wg.Wait()
}

func processCommitRemote(ctx context.Context, cs *storage.Client, bucket *storage.BucketHandle, commit Commit, runChan chan protos.TestRun, resultChan chan protos.TestResult, errChan chan error) {
	shortHash := commit.shortHash
	it := bucket.Objects(ctx, &storage.Query{
		Delimiter: "/",
		Prefix: shortHash,
	})
	var wg sync.WaitGroup
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			errChan <- err
			continue
		}
		// Prefix only set on directories, which is what we
		// seek.
		if attrs.Prefix != "" {
			// Drop trailing slash
			log.Println("+++++++++++++++++++++++++++++++++" + attrs.Prefix)
			platformStr := attrs.Prefix[:len(attrs.Prefix)-1]
			var testRun protos.TestRun
			if err := addPlatformToTestRun(platformStr, &testRun); err != nil {
				errChan <- err
			} else {
				runChan <- testRun
				wg.Add(1)
				go func() {
					defer wg.Done()
					processTestRunResultsRemote(ctx, cs, bucket, shortHash, platformStr, testRun, resultChan, errChan)
				}()
			}
		}
	}
	wg.Wait()
}

func processCommitsRemote(ctx context.Context, cs *storage.Client, bucket *storage.BucketHandle, commits []Commit) (runChan chan protos.TestRun, resultChan chan protos.TestResult, errChan chan error) {
	runChan = make(chan protos.TestRun)
	resultChan = make(chan protos.TestResult)
	errChan = make(chan error)
	var wg sync.WaitGroup
	wg.Add(len(commits))
	for _, commit := range commits {
		go func(c Commit) {
			defer wg.Done()
			processCommitRemote(ctx, cs, bucket, c, runChan, resultChan, errChan)
		}(commit)
	}

	go func() {
		defer close(runChan)
		defer close(resultChan)
		defer close(errChan)
		wg.Wait()
	}()

	return runChan, resultChan, errChan
}

func getCommitsLocal(wptPath *string, dataPath *string) (commits []Commit) {
	hashes, err := hashesFromDataPath(*dataPath)
	if err != nil {
		log.Fatal(err)
	}
	commitPtrs := dropNilCommits(hashesToCommits(*wptPath, hashes))
	sort.Sort(sort.Reverse(ByCommitTime(commitPtrs)))
	for _, commit := range commitPtrs {
		commits = append(commits, *commit)
	}
	return commits
}

func processCommitsLocal(dataPath *string, commits []Commit) {
	// ctx := context.Background()
	// client, err := datastore.NewClient(ctx, projectId)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// testResultChan := make(chan protos.TestResult)
	for _, commit := range commits {
		testRuns, err := testRunsFromDataPath(*dataPath, commit.shortHash)
		if err != nil {
			log.Fatal(err)
		}
		for _, testRun := range testRuns {
			if err := addCommitToTestRun(commit, &testRun.testRun); err != nil {
				log.Fatal(err)
			}

			// createdAt, err := createdAtFromShortHashDatastore(ctx, client, commit.shortHash)
			// if err != nil {
			// 	log.Fatal(err)
			// }

			findErrors := make(chan error)
			find := makeChanCmd(nil, "find", *dataPath + "/" + commit.shortHash + "/" + testRun.platformStr, "-type", "f")
			find.Start(findErrors)
			find.Wait(findErrors)

			// TODO: Write this
			// catAndDecodeFiles(testRun.testRun, find.stdoutChan, testResultChan)

			// TODO: Should spawn goroutines for each element,
			// rather than waiting for close(chan), as range chan
			// does.
			entries := make([]string, 0)
			for entry := range find.stdoutChan {
				entries = append(entries, entry)
			}
			for err := range findErrors {
				log.Fatal(err)
			}
			log.Println(entries)
		}
	}

	// TODO: Consume testResultChan
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	// gsUrl := flag.String("gs_url", "gs://wptd", "Google Cloud Storage URL that is parent directory to git hash directories")
	wptPath := flag.String("wpt_path", os.Getenv("HOME")+"/web-platform-tests", "Path to Web Platform Tests repository")
	// dataPath := flag.String("data_path", os.Getenv("HOME")+"/wpt-data", "Path to data directory for local data copied from Google Cloud Storage")
	projectId := flag.String("project_id", "wptdashboard", "Google Cloud Platform project id")

	ctx := context.Background()
	ds, err := datastore.NewClient(ctx, *projectId)
	if err != nil {
		log.Fatal(err)
	}
	cs, err := storage.NewClient(ctx, option.WithoutAuthentication())
	bucket := cs.Bucket("wptd")

	commits := getCommitsRemote(wptPath, ctx, ds, cs, bucket)
	runChan, resultChan, errChan := processCommitsRemote(ctx, cs, bucket, commits)

	var wg sync.WaitGroup
	wg.Add(3)
	go func(c chan protos.TestRun) {
		defer wg.Done()
		for v := range c {
			log.Println(v)
		}
	}(runChan)
	go func(c chan protos.TestResult) {
		defer wg.Done()
		for v := range c {
			log.Println(v)
		}
	}(resultChan)
	go func(c chan error) {
		defer wg.Done()
		for v := range c {
			log.Println(v)
		}
	}(errChan)
	wg.Wait()

	// commits := getCommitsLocal(wptPath, dataPath)
	// processCommitsLocal(dataPath, commits)
}
