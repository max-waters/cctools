package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/pkg/errors"
)

func LoadControllerValues(filename string) ([]*ControllerValue, error) {
	rows := []*ControllerValue{}
	if err := UnmarshalCsv(filename, &rows); err != nil {
		return nil, errors.Wrap(err, "cannot load controller values")
	}
	return rows, nil
}

func LoadVoiceControllerValues(filename string) ([]*VoiceControllerValue, error) {
	rows := []*VoiceControllerValue{}
	if err := UnmarshalCsv(filename, &rows); err != nil {
		return nil, errors.Wrap(err, "cannot load controller values")
	}
	return rows, nil
}

func UnmarshalCsv(filename string, dest interface{}) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return gocsv.Unmarshal(f, dest)
}

func SaveVoiceControllerValues(filename string, data []*VoiceControllerValue) (string, error) {
	filename, err := WriteDataToFile(filename, data, gocsv.Marshal)
	if err != nil {
		return "", errors.Wrap(err, "cannot save voice controller values")
	}
	return filename, nil
}

func SaveVoiceControllerValuesAsMaxMsp(filename string, data []*VoiceControllerValue) (string, error) {
	dataTable, err := FormatVoiceControllerValues(data)
	if err != nil {
		return "", nil
	}

	filename, err = WriteDataToFile(filename, dataTable, WriteMaxMspCollFormat)
	if err != nil {
		return "", errors.Wrap(err, "cannot save voice controller values in Max/MSP format")
	}
	return filename, nil
}

// data: voice x variation x ctrler x value
func SaveVoiceVariationControllerValuesAsMaxMsp(filename string, data [][]*VoiceControllerValue) (string, error) {
	tableData := [][]interface{}{}
	for i, vcvs := range data {
		formatted, err := FormatVoiceControllerValues(vcvs)
		if err != nil {
			return "", err
		}

		// append variation num to other rows
		for j := 0; j < len(formatted); j++ {
			row := append([]interface{}{i}, formatted[j]...)
			tableData = append(tableData, row)
		}
	}

	return WriteDataToFile(filename, tableData, WriteMaxMspTextFormat)
}

func FormatVoiceControllerValues(data []*VoiceControllerValue) ([][]interface{}, error) {
	voiceSet := map[uint8]interface{}{}
	controllerSet := map[uint8]interface{}{}

	// put data into map controller -> voice -> value
	dataMap := map[uint8]map[uint8]uint8{}
	for _, d := range data {
		voiceSet[d.Voice] = nil
		controllerSet[d.Controller] = nil

		controllerMap, ok := dataMap[d.Controller]
		if !ok {
			controllerMap = map[uint8]uint8{}
			dataMap[d.Controller] = controllerMap
		}

		controllerMap[d.Voice] = d.Value
	}

	// check all controllers have the same number of voices
	size := -1
	for v, controllerMap := range dataMap {
		if size == -1 {
			size = len(controllerMap)
		} else if len(controllerMap) != size {
			return nil, errors.Errorf("Voice %d has %d controller values, expected %d", v, len(controllerMap), size)
		}
	}

	// get unique voices and controllers
	voices := OrderSet(voiceSet)
	controllers := OrderSet(controllerSet)

	// remove controllers that do not vary
	filteredControllers := []uint8{}
	for _, controller := range controllers {
		val := -1
		diff := false
		for _, voice := range voices {
			if val == -1 {
				val = int(dataMap[controller][voice])
			} else if int(dataMap[controller][voice]) != val {
				diff = true
				break
			}
		}

		if diff {
			filteredControllers = append(filteredControllers, controller)
		}
	}
	controllers = filteredControllers

	// create table with headers row
	dataTable := make([][]interface{}, len(controllers))
	dataTable[0] = make([]interface{}, len(voices)+1)

	for i, controller := range controllers {
		dataTable[i] = make([]interface{}, len(voices)+1)
		dataTable[i][0] = controller

		for j, voice := range voices {
			dataTable[i][j+1] = dataMap[controller][voice]
		}
	}

	return dataTable, nil
}

func WriteMaxMspTextFormat(in interface{}, out io.Writer) error {
	dataTable, ok := in.([][]interface{})
	if !ok {
		return errors.Errorf("must be of type [][]interface{}, not %T", in)
	}

	for _, row := range dataTable {
		for _, val := range row {
			out.Write([]byte(fmt.Sprintf("%v ", val)))
		}
		out.Write([]byte("\n"))
	}
	return nil
}

func WriteMaxMspCollFormat(in interface{}, out io.Writer) error {
	dataTable, ok := in.([][]interface{})
	if !ok {
		return errors.Errorf("must be of type [][]interface{}, not %T", in)
	}

	for _, row := range dataTable {
		for j, val := range row {
			out.Write([]byte(fmt.Sprintf("%v", val)))
			if j == 0 {
				out.Write([]byte(","))
			}
			out.Write([]byte(" "))
		}
		out.Write([]byte(";\n"))
	}

	return nil
}

func SaveControllerValues(filename string, data []*ControllerValue) (string, error) {
	filename, err := WriteDataToFile(filename, &data, gocsv.Marshal)
	if err != nil {
		return "", errors.Wrap(err, "cannot save controller values")
	}
	return filename, nil
}

func LoadSysEx(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func SaveSysex(filename string, data []byte) (string, error) {
	filename, err := WriteDataToFile(filename, data, writeBytes)
	if err != nil {
		return "", errors.Wrap(err, "cannot save sysex data")
	}
	return filename, nil
}

func writeBytes(in interface{}, out io.Writer) error {
	bts, ok := in.([]byte)
	if !ok {
		return fmt.Errorf("not a []byte: %T", in)
	}
	_, err := out.Write(bts)
	return err
}

func WriteDataToFile(filename string, data interface{}, writeFunc func(in interface{}, out io.Writer) error) (string, error) {
	filename = FormatFileName(filename)
	if err := BackupIfExists(filename); err != nil {
		return "", errors.Wrap(err, "cannot back up file")
	}

	// create parent directories
	if err := os.MkdirAll(filepath.Dir(filename), os.ModePerm); err != nil {
		return "", err
	}

	// create file
	f, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// write data
	return filename, writeFunc(data, f)
}

func BackupIfExists(filename string) error {
	// if it is a new file, return
	if _, err := os.Stat(filename); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} else {
			return errors.Wrapf(err, "cannot check if file '%s' exists", filename)
		}
	}

	// make backip dir
	dir := filepath.Dir(filename) + "/bak"
	if err := os.MkdirAll(dir, 0777); err != nil {
		return errors.Wrapf(err, "cannot create backup directory '%s'", dir)
	}

	// find backup file name
	extension := filepath.Ext(filename)
	fileBase := strings.TrimSuffix(filepath.Base(filename), extension)

	regex, err := GetNumberedFileRegex(fileBase, extension)
	if err != nil {
		return err
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrapf(err, "cannot read contents of directory '%s'", dir)
	}

	highest := -1
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		matches := regex.FindStringSubmatch(file.Name())
		if len(matches) == 0 {
			continue
		}

		n, err := strconv.Atoi(matches[1])
		if err != nil {
			return err
		}
		if n > highest {
			highest = n
		}
	}
	backupFileName := fmt.Sprintf("%s/%s-%03d%s", dir, fileBase, highest+1, extension)

	// copy current file to backup
	source, err := os.Open(filename)
	if err != nil {
		return errors.Wrapf(err, "cannot open file '%s'", filename)
	}
	defer source.Close()

	destination, err := os.Create(backupFileName)
	if err != nil {
		return errors.Wrapf(err, "cannot open file '%s'", backupFileName)
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return errors.Wrap(err, "cannot copy to backup file")
	}

	return nil
}

func FormatFileName(filename string) string {
	if ext := filepath.Ext(filename); len(ext) > 0 { // file has an extension, keep it
		return filename
	}
	return filename + ".csv"
}

func GetNumberedFileRegex(filename, extension string) (*regexp.Regexp, error) {
	if len(extension) > 0 && extension[0] != '.' {
		extension = "." + extension
	}
	sanitisedFileName := regexp.QuoteMeta(filename)
	sanitisedExt := regexp.QuoteMeta(extension)
	return regexp.Compile(fmt.Sprintf("^%s-([\\d]+)%s$", sanitisedFileName, sanitisedExt))
}

func OrderSet(set map[uint8]interface{}) []uint8 {
	ordered := make([]uint8, len(set))
	i := 0
	for n := range set {
		ordered[i] = n
		i++
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return ordered
}
