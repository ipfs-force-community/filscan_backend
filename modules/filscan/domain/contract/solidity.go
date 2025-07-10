package contract

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
)

func NewSolidity(path string) (*Solidity, error) {
	if path == "" {
		path = "solc"
	}
	var stdOut bytes.Buffer
	cmd := exec.Command(path, "--version")
	cmd.Stdout = &stdOut
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	var versionRegexp = regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)`)
	matches := versionRegexp.FindStringSubmatch(stdOut.String())
	if len(matches) != 4 {
		return nil, fmt.Errorf("can't parse solc version %q", stdOut.String())
	}
	s := &Solidity{Path: cmd.Path, FullVersion: stdOut.String(), Version: matches[0]}
	s.Path = path
	if s.Major, err = strconv.Atoi(matches[1]); err != nil {
		return nil, err
	}
	if s.Minor, err = strconv.Atoi(matches[2]); err != nil {
		return nil, err
	}
	if s.Patch, err = strconv.Atoi(matches[3]); err != nil {
		return nil, err
	}
	return s, nil
}

type Solidity struct {
	Path        string
	Version     string
	FullVersion string
	Major       int
	Minor       int
	Patch       int
	//ExtraAllowedPath []string
}

//func (s *Solidity) allowedPaths() string {
//	paths := []string{".", "./", "../"} // default to support relative paths
//	if len(s.ExtraAllowedPath) > 0 {
//		paths = append(paths, s.ExtraAllowedPath...)
//	}
//	return strings.Join(paths, ", ")
//}

func (s *Solidity) MakeArgs(filePath string, optimize *Optimize, isMetaData bool) []string {
	p := []string{
		filePath,
		"--combined-json",
		"bin,abi,userdoc,devdoc",
		//"--optimize",                  // code optimizer switched on
		//"--allow-paths", "../", // default to support relative paths
	}
	if s.Major > 0 || s.Minor > 4 || s.Patch > 6 {
		p[2] += ",metadata,hashes"
	}
	if optimize != nil && optimize.Enabled == true {
		runs := strconv.FormatInt(optimize.Runs, 10)
		p = append(p, "--optimize")
		p = append(p, "--optimize-runs")
		p = append(p, runs)
	}
	if isMetaData == true {
		p = append(p, "--allow-paths")
		p = append(p, "../")
	}
	return p
}

func (s *Solidity) ChangeVersion(path string, target string) error {
	if path == "" {
		path = "solc-select"
	}
	var stderr, stdout bytes.Buffer
	cmd := exec.Command(path, "use", target, "--always-install")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	fmt.Printf("solidity: %s", stderr.String())
	fmt.Printf("solidity: %s", stdout.String())
	return nil
}

func (s *Solidity) String() error {
	marshal, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	err = json.Indent(&out, marshal, "", "\t")
	if err != nil {
		return err
	}
	fmt.Printf("solidity=%v\n", out.String())
	return nil
}

func (s *Solidity) GetVersionList(filePath string) (list []string, err error) {
	versionList, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	buf := bufio.NewScanner(versionList)
	for {
		if !buf.Scan() {
			break
		}
		line := buf.Text()
		fmt.Println(line)
		list = append(list, line)
	}
	return
}
