package main

import (
	"bufio"
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

func gsutilLs(args ...string) (entries []string, err error) {
	cmd := makeCmd(nil, append(append(append(make([]string, 0), "gsutil"), "ls"), args...)...)
	if err != nil {
		return entries, err
	}
	if err := cmd.Start(); err != nil {
		return entries, err
	}
	scanner := bufio.NewScanner(cmd.stdout)
	entries = make([]string, 0)
	for scanner.Scan() {
		entries = append(entries, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, err
}

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

func shortHashToCommit(wptPath string, shortHash string) (commit *Commit) {
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

func gsutilCp(src string, dst string) (err error) {
	cmd := makeCmd(nil, "gsutil", "cp", "-r", "-n", src, dst)
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	gsUrl := flag.String("gs_url", "gs://wptd", "Google Cloud Storage URL that is parent directory to git hash directories")
	wptPath := flag.String("wpt_path", os.Getenv("HOME")+"/web-platform-tests", "Path to Web Platform Tests repository")
	dataPath := flag.String("data_path", os.Getenv("HOME")+"/wpt-data", "Path to data directory for local data copied from Google Cloud Storage")

	if _, err := os.Stat(*dataPath); os.IsNotExist(err) {
		os.Mkdir(*dataPath, os.ModePerm)
	}

	entries, err := gsutilLs(*gsUrl)
	if err != nil {
		log.Fatal(err)
	}

	hashes := filterGsUrlsToHashes(entries)
	commits := dropNilCommits(hashesToCommits(*wptPath, hashes))
	sort.Sort(sort.Reverse(ByCommitTime(commits)))

	// To test, just try a couple
	commits = commits[:1]

	// for _, commit := range commits {
	// 	src := *gsUrl + "/" + commit.shortHash
	// 	dst := *dataPath + "/" + commit.shortHash

	// 	if _, err := os.Stat(dst); os.IsNotExist(err) {
	// 		os.Mkdir(dst, os.ModePerm)
	// 	}

	// 	gsutilCp(src, dst)
	// }
	for _, commit := range commits {
		remotePath := *gsUrl + "/" + commit.shortHash
		entries, err := gsutilLs("-r", remotePath)
		if err != nil {
			log.Fatal(err)
		}
		for _, entry := range entries {
			if len(entry) <= len(remotePath) {
				log.Printf("Short path  : %s\n", entry)
			} else {
				log.Printf("Clipped path: %s\n", entry[len(remotePath):])
			}
		}
	}
}
