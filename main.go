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

func deepCopyCssFile(source cssFile) cssFile {
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

func reduceCSS(fileName string, parsed chan cssFile) error {
	var parsedCSS cssFile
	parsedCSS.classes = make(map[string][]string)
	cutset := "\r\n \t;"
	rawText, err := ioutil.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("unable to read file %s - %s\n", fileName, err.Error())
	}
	textString := strings.Trim(string(rawText), cutset)
	for len(textString) > 0 {
		leftBracket := strings.Index(textString, "{")
		rightBracket := strings.Index(textString, "}")
		classNames := strings.Trim(textString[0:leftBracket-1], cutset)
		classDef := strings.Trim(textString[leftBracket+1:rightBracket-1], cutset)
		classAttr := strings.Split(classDef, ";")
		for i, item := range classAttr {
			classAttr[i] = strings.Trim(item, cutset)
		}
		sort.Strings(classAttr)
		classDef = strings.Join(classAttr, ";\n") + ";\n"
		parsedCSS.classes[classDef] = append(parsedCSS.classes[classDef], classNames)
		textString = strings.Trim(textString[rightBracket+1:], cutset)
	}
	parsedCSS.fileName = fileName
	fmt.Printf("adding %s to the parsed chan\n", fileName)
	parsed <- parsedCSS
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
	extractedFile := deepCopyCssFile(parsedFiles[0])
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
	task := make(chan string)
	parsed := make(chan cssFile)
	save := make(chan cssFile)
	taskWG.Add(*maxWorkers)
	for w := 0; w < *maxWorkers; w++ {
		go func() {
			defer taskWG.Done()
			for file := range task {
				myErr := reduceCSS(file, parsed)
				if myErr != nil {
					fmt.Print(myErr.Error())
				}
			}
		}()
	}

	saveWG.Add(*maxWorkers)
	for w := 0; w < *maxWorkers; w++ {
		go func() {
			defer saveWG.Done()
			for cssFileData := range save {
				myErr := saveCSS(cssFileData)
				if myErr != nil {
					fmt.Println(myErr.Error())
				}
			}
		}()
	}

	// process files
	cssCount := 0
	go func() {
		for _, file := range files {
			if strings.HasSuffix(strings.ToLower(file.Name()), ".css") {
				fmt.Printf("%s added to target group\n", file.Name())
				cssCount++
				task <- *dir + "/" + file.Name()
			}
		}
		close(task)
	}()
	//taskWG.Wait()
	parsedFiles := make([]cssFile,0)
	for i := 0; i < cssCount; i++ {
		parsedFiles = append(parsedFiles, <-parsed)
	}
	close(parsed)
	fmt.Println("extracting common styles")
	processedFiles := extractCommonStyles(parsedFiles)
	for _, completedFile := range processedFiles {
		save <- completedFile
	}
	close(save)
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
