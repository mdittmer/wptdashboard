package main

import (
	"bufio"
	"encoding/json"
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
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
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
	commitChan := make(chan *Commit)
	var wg sync.WaitGroup
	wg.Add(len(hashes))
	for _, hash := range hashes {
		go func(shortHash string) {
			defer wg.Done()
			commitChan <- shortHashToCommit(wptPath, shortHash)
		}(hash)
	}

	commits = make([]*Commit, 0)
	go func() {
		for commit := range commitChan {
			commits = append(commits, commit)
		}
	}()
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
	Message string `json:"message"`
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

func hashesFromDatastore(projectId string) (hashes []string, err error) {
	ctx := context.Background()
	client, err := datastore.NewClient(ctx, projectId)
	if err != nil {
		return hashes, err
	}
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

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	gsUrl := flag.String("gs_url", "gs://wptd", "Google Cloud Storage URL that is parent directory to git hash directories")
	wptPath := flag.String("wpt_path", os.Getenv("HOME")+"/web-platform-tests", "Path to Web Platform Tests repository")
	dataPath := flag.String("data_path", os.Getenv("HOME")+"/wpt-data", "Path to data directory for local data copied from Google Cloud Storage")
	projectId := flag.String("project_id", "wptdashboard", "Google Cloud Platform project id")

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
		if _, err := os.Stat(*dataPath); os.IsNotExist(err) {
			os.Mkdir(*dataPath, os.ModePerm)
		}

		hashesDS, err := hashesFromDatastore(*projectId)
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
		gsutilLsErrors := make(chan error)
		gsutilLs := makeChanCmd(nil, "gsutil", "ls", *gsUrl)
		gsutilLs.Start(gsutilLsErrors)
		gsutilLs.Wait(gsutilLsErrors)

		entries := make([]string, 0)
		for entry := range gsutilLs.stdoutChan {
			entries = append(entries, entry)
		}
		for err := range gsutilLsErrors {
			log.Fatal(err)
		}

		hashes := filterGsUrlsToHashes(entries)
		commitsCS = dropNilCommits(hashesToCommits(*wptPath, hashes))
		sort.Sort(sort.Reverse(ByCommitTime(commitsCS)))
	}()

	wg.Wait()
	goodRuns := make([]Commit)
	for i, j := 0, 0; i < len(commitsDS) && j < len(commitsCS); {
		cmp := strings.Compare(commitsDS[i].shortHash, commitsCS[j].shortHash)
		if cmp < 0 {
			log.Printf("Lone DS: %s", commitsDS[i])
			i++
		} else if cmp > 0 {
			log.Printf("Lone CS: %s", commitsCS[j])
			j++
		} else {
			goodRuns = append(goodRuns, *commitDS[i])
			i++
			j++
		}
	}

	// for _, commit := range commits {
	// 	log.Println(commit)
	// }
}
