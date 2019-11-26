package server

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/biogo/hts/bam"
	"github.com/david-xliu/htsget-refserver/internal/genomics"
	"github.com/go-chi/chi"
)

// getData serves the actual data from AWS back to client
func getData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// *** Parse query params ***
	params := r.URL.Query()
	format, err := parseFormat(params)
	if format != "BAM" {
		panic("format not supported")
	}
	refName, err := parseRefName(params)
	start, end, err := parseRange(params, refName)
	fields, err := parseFields(params)
	blockID, _ := strconv.Atoi(r.Header.Get("block-id"))
	numBlocks, _ := strconv.Atoi(r.Header.Get("num-blocks"))
	class := r.Header.Get("class")

	region := &genomics.Region{Name: refName, Start: start, End: end}

	args := getCmdArgs(id, region, numBlocks, class)
	cmd := exec.Command("samtools", args...)

	pipe, _ := cmd.StdoutPipe()
	err = cmd.Start()
	if err != nil {
		panic(err)
	}
	tempPath := getTempPath(id, blockID)

	fSam, _ := os.Create(tempPath)
	defer fSam.Close()
	reader := bufio.NewReader(pipe)

	// write header and return
	if class == "header" {
		io.Copy(fSam, reader)
		if blockID == numBlocks {
			removeEOF(tempPath)
		}
		io.Copy(w, fSam)
		cmd.Wait()
		return
	}

	if len(fields) == 0 {
		cwd, _ := os.Getwd()
		l := reader.ReadString("/n")
		os.Create()
	} else {
		//processData(fSam, reader, refName, fields)
		l, _, err := reader.ReadLine()
		columns := make([]bool, 11)
		for _, field := range fields {
			columns[FIELDS[field]-1] = true
		}

		for ; err == nil; l, _, err = reader.ReadLine() {
			if l[0] != 64 {
				var output []string
				ls := strings.Split(string(l), "\t")

				fmt.Println("test")
				for i, col := range columns {
					if col {
						output = append(output, ls[i])
					} else {
						if i == 1 || i == 3 || i == 4 || i == 7 || i == 8 {
							output = append(output, "0")
						} else {
							output = append(output, "*")
						}
					}
				}
				l = []byte(strings.Join(output, "\t") + "\n")

				fSam.Write(l)

				samToBam(tempPath)
				removeHeader(tempPath + "_bam")
				removeEOF(tempPath + "_bam")

				fclient, _ := os.Open(tempPath + "_bam")
				defer fclient.Close()
				fmt.Println("writing data")
				io.Copy(w, fclient)
				fmt.Println("wrote data")
			}
		}

		if blockID == numBlocks {
			w.Write(EOF)
		}
	}
	cmd.Wait()

	/* samToBam(tempPath)*/
	/*trimBlob(class, numBlocks, tempPath, blockID)*/

	/* fclient, _ := os.Open(tempPath + "_bam")*/
	//defer fclient.Close()
	/*io.Copy(w, fclient)*/
}

/*func processData(fSam *os.File, r *bufio.Reader, name string, fields []string) {*/
//l, _, err := r.ReadLine()
//columns := make([]bool, 11)
//for _, field := range fields {
//columns[FIELDS[field]-1] = true
//}

//for ; err == nil; l, _, err = r.ReadLine() {
//if l[0] == 64 {
//l = append(l, "\n"...)
//fSam.Write(l)
//} else {
//var output []string
//ls := strings.Split(string(l), "\t")
//keepLine := true
//if name == "*" {
//keepLine = isUnmappedUnplaced(ls)
//}
//if keepLine {
//for i, col := range columns {
//if col {
//output = append(output, ls[i])
//} else {
//if i == 1 || i == 3 || i == 4 || i == 7 || i == 8 {
//output = append(output, "0")
//} else {
//output = append(output, "*")
//}
//}
//}
//l = []byte(strings.Join(output, "\t") + "\n")
//fSam.Write(l)
//}
//}
//}
/*}*/

func getTempPath(id string, blockID int) string {
	cwd, _ := os.Getwd()
	parent := filepath.Dir(cwd)
	tempPath := parent + "/temp/" + id + "_" + strconv.Itoa(blockID)

	if exists, _ := fileExists(parent + "/temp"); !exists {
		os.Mkdir(parent+"/temp/", 0755)
	} else {
		if isDir, _ := isDir(parent + "/temp"); !isDir {
			os.Mkdir(parent+"/temp/", 0755)
		}
	}
	return tempPath
}

func getCmdArgs(id string, r *genomics.Region, numBlocks int, class string) []string {
	args := []string{"view", dataSource + filePath(id)}
	if class == "header" {
		args = append(args, "-H")
		args = append(args, "-b")
	} else {
		if r.String() != "" {
			args = append(args, r.String())
		}
		args = append(args, "-h")
	}
	return args
}

func samToBam(tempPath string) {
	cmd := exec.Command("samtools", "view", "-h", "-b", tempPath, "-o", tempPath+"_bam")
	cmd.Run()
}

func removeHeader(tempPath string) {
	fin, _ := os.Open(tempPath)
	defer fin.Close()
	b, _ := bam.NewReader(fin, 0)
	defer b.Close()
	b.Header()
	b.Read()
	lastChunk := b.LastChunk()
	hLen := lastChunk.Begin.File

	fDest, _ := os.Create(tempPath + "_copy")
	fin.Seek(hLen, 0)
	io.Copy(fin, fDest)

	os.Remove(tempPath)
	os.Rename(tempPath+"_copy", tempPath)
}

func removeEOF(tempPath string) error {
	fi, _ := os.Stat(tempPath)
	return os.Truncate(tempPath, fi.Size()-12)
}

// exists returns whether the given file or directory existsi.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil || !os.IsNotExist(err) {
		return true, err
	}
	return false, nil
}

// isDir returns whether the given path is directory
func isDir(path string) (bool, error) {
	src, err := os.Stat(path)

	if src.Mode().IsRegular() {
		fmt.Println(path + " already exist as a file!")
		return false, err
	}
	return true, err
}

/*func isUnmappedUnplaced(l []string) bool {*/
//flag, _ := strconv.ParseInt(l[1], 2, 64)
//flag = flag >> 2
//unmapped := flag&1 == 1

//return unmapped && l[2] == "*" && l[3] == "0"
/*}*/

func trimBlob(class string, numBlocks int, tempPath string, blockID int) {
	if class == "header" && numBlocks > 1 {
		removeEOF(tempPath + "_bam")
	} else if class == "body" {
		removeHeader(tempPath + "_bam")
		if blockID != numBlocks {
			removeEOF(tempPath + "_bam")
		}
	}
}
