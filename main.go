package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
)

type cssFile struct {
	fileName string
	classes  map[string][]string
}

func (source cssFile) deepCopyCssFile() cssFile {
	extractedFile := source
	extractedFile.classes = map[string][]string{}
	for key, value := range source.classes {
		extractedFile.classes[key] = make([]string, len(value))
		copy(extractedFile.classes[key], value)
	}
	return extractedFile
}

func mapContains(baseClasses map[string][]string, key string, value []string) bool {
	//for key, value := range baseClasses {
	//
	//}
	if names, ok := baseClasses[key]; ok {
		return reflect.DeepEqual(names, value)
	}
	return false
}

func filterFile(baseFile cssFile, filterFile cssFile) cssFile {
	var result cssFile
	result.fileName = baseFile.fileName
	result.classes = make(map[string][]string, 0)
	for key, value := range baseFile.classes {
		if _, ok := filterFile.classes[key]; !ok {
			result.classes[key] = value
		}
	}
	return result
}

type cssChannel struct {
	c    chan cssFile
	once sync.Once
}

func newCssChannel() *cssChannel {
	return &cssChannel{c: make(chan cssFile)}
}

func (cc *cssChannel) closeSafe() {
	cc.once.Do(func() {
		close(cc.c)
	})
}

func reduceCSS(fileName string, parsed *cssChannel) error {
	var parsedCSS cssFile
	parsedCSS.classes = make(map[string][]string)
	cutSet := "\r\n \t;"
	rawText, err := ioutil.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("unable to read file %s - %s\n", fileName, err.Error())
	}
	textString := strings.Trim(string(rawText), cutSet)
	for len(textString) > 0 {
		leftBracket := strings.Index(textString, "{")
		rightBracket := strings.Index(textString, "}")
		classNames := strings.Trim(textString[0:leftBracket-1], cutSet)
		classDef := strings.Trim(textString[leftBracket+1:rightBracket-1], cutSet)
		classAttr := strings.Split(classDef, ";")
		for i, item := range classAttr {
			classAttr[i] = strings.Trim(item, cutSet)
		}
		sort.Strings(classAttr)
		classDef = strings.Join(classAttr, ";\n") + ";\n"
		parsedCSS.classes[classDef] = append(parsedCSS.classes[classDef], classNames)
		textString = strings.Trim(textString[rightBracket+1:], cutSet)
	}
	parsedCSS.fileName = fileName
	fmt.Printf("adding %s to the parsed chan\n", fileName)
	parsed.c <- parsedCSS
	return nil
}

func saveCSS(file cssFile) error {
	buf := &bytes.Buffer{}
	for classDef, classNames := range file.classes {
		buf.WriteString(strings.Join(classNames, ", "))
		buf.WriteString(" {\n")
		buf.WriteString(classDef)
		buf.WriteString("}\n\n")
	}
	newFileName := file.fileName + ".new"
	fmt.Printf("processed %s and now saving to file %s\n", file.fileName, newFileName)
	return ioutil.WriteFile(newFileName, buf.Bytes(), 0664)
}

func extractCommonStyles(parsedFiles []cssFile) []cssFile {
	var result []cssFile
	var refactored cssFile
	refactored.fileName = "refactored.css"
	refactored.classes = map[string][]string{}
	extractedFile := parsedFiles[0].deepCopyCssFile()
	for key, value := range extractedFile.classes {
		extracting := true
		for i := 0; i < len(parsedFiles); i++ {
			if extracting {
				extracting = mapContains(parsedFiles[i].classes, key, value)
			}
		}
		if extracting {
			refactored.classes[key] = value
		}
	}
	result = append(result, refactored)
	for _, unfactoredFile := range parsedFiles {
		result = append(result, filterFile(unfactoredFile, refactored))
	}
	return result
}

func main() {
	// parse flags
	dir := flag.String("dir", ".", "directory containing css files to refactor")
	maxWorkers := flag.Int("workers", 5, "max number of concurrent workers to process files")
	flag.Parse()

	// collect file names
	files, err := loadDir(*dir)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// parse of each file
	var taskWG, saveWG sync.WaitGroup
	task := newCssChannel()
	parsed := newCssChannel()
	save := newCssChannel()
	taskWG.Add(*maxWorkers)
	for w := 0; w < *maxWorkers; w++ {
		go func() {
			for file := range task.c {
				myErr := reduceCSS(file.fileName, parsed)
				if myErr != nil {
					fmt.Print(myErr.Error())
				}
			}
			taskWG.Done()
			taskWG.Wait()
			parsed.closeSafe()
		}()
	}

	saveWG.Add(*maxWorkers)
	for w := 0; w < *maxWorkers; w++ {
		go func() {
			defer saveWG.Done()
			for cssFileData := range save.c {
				myErr := saveCSS(cssFileData)
				if myErr != nil {
					fmt.Println(myErr.Error())
				}
			}
		}()
	}

	// process files
	go func() {
		for _, file := range files {
			if strings.HasSuffix(strings.ToLower(file.Name()), ".css") {
				fmt.Printf("%s added to target group\n", file.Name())
				targetFile := cssFile{fileName: *dir + "/" + file.Name()}
				task.c <- targetFile
			}
		}
		task.closeSafe()
	}()

	//taskWG.Wait()
	parsedFiles := make([]cssFile, 0)
	//for i := 0; i < cssCount; i++ {
	for parsedFile := range parsed.c {
		parsedFiles = append(parsedFiles, parsedFile)
	}
	fmt.Println("extracting common styles")
	processedFiles := extractCommonStyles(parsedFiles)
	for _, completedFile := range processedFiles {
		save.c <- completedFile
	}
	save.closeSafe()
	saveWG.Wait()
}

func loadDir(dir string) ([]os.FileInfo, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("unable to open given directory - %s\n", err.Error())
	}
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve list of files in the %s diectory - %s\n", dir, err.Error())
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in the %s directory\n", dir)
	}
	return files, nil
}
