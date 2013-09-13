package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"flag"
	"io/ioutil"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"path/filepath"
)

type Os struct{}

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()
	o := &protocol.Options{Username: "fatih", Kitename: "os", Version: "1", Port: *port}
	k := kite.New(o, new(Os))
	k.Start()
}

func (Os) ReadDirectory(r *protocol.KiteRequest, result *[]string) error {
	path := r.Args.(string)
	files, err := ReadDirectory(path)
	if err != nil {
		return err
	}

	*result = files
	return nil
}

func (Os) Glob(r *protocol.KiteRequest, result *[]string) error {
	glob := r.Args.(string)
	files, err := Glob(glob)
	if err != nil {
		return err
	}

	*result = files

	return nil
}

func (Os) ReadFile(r *protocol.KiteRequest, result *map[string]interface{}) error {
	path := r.Args.(string)
	buf, err := ReadFile(path)
	if err != nil {
		return err
	}

	*result = map[string]interface{}{"content": buf}
	return nil
}

func (Os) WriteFile(r *protocol.KiteRequest, result *string) error {
	// TODO: write an Unmarshaller from interface{} to a given struct
	params := r.Args.(map[string]interface{})
	path, ok := params["path"].(string)
	if !ok {
		return errors.New("path argument missing")
	}
	content, ok := params["content"].(string)
	if !ok {
		return errors.New("content argument missing")
	}
	doNotOverwrite, _ := params["doNotOverwrite"].(bool)
	appendTo, _ := params["append"].(bool)

	buf, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return err
	}

	err = WriteFile(path, buf, doNotOverwrite, appendTo)
	if err != nil {
		return err
	}

	*result = fmt.Sprintf("content written to %s", path)
	return nil
}

func (Os) EnsureNonexistentPath(r *protocol.KiteRequest, result *string) error {
	name := r.Args.(string)
	name, err := EnsureNonexistentPath(name)
	if err != nil {
		return err
	}

	*result = name
	return nil
}

func (Os) GetInfo(r *protocol.KiteRequest, result *FileEntry) error {
	path := r.Args.(string)

	fileEntry, err := GetInfo(path)
	if err != nil {
		return err
	}

	*result = *fileEntry
	return nil
}

func (Os) SetPermissions(r *protocol.KiteRequest, result *bool) error {
	params := r.Args.(map[string]interface{})
	path, ok := params["path"].(string)
	if !ok {
		return errors.New("path argument missing")
	}
	mode, ok := params["mode"].(int)
	if !ok {
		return errors.New("mode argument missing")
	}
	recursive, ok := params["recursive"].(bool)
	if !ok {
		return errors.New("recursive argument missing")
	}

	err := SetPermissions(path, os.FileMode(mode), recursive)
	if err != nil {
		return err
	}

	*result = true
	return nil

}

/****************************************
*
* Make the functions below to a seperate package
*
*****************************************/
func unmarshal(a, s interface{}) {
	t := reflect.TypeOf(s)
	if t.Kind() != reflect.Struct {
		fmt.Printf("%v type can't have attributes inspected\n", t.Kind())
		return
	}

	params := make(map[string]reflect.Type)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		params[field.Name] = field.Type
	}

	x := reflect.TypeOf(a)
	if x.Kind() != reflect.Map {
		fmt.Printf("%v type can't have attributes inspected\n", x.Kind())
		return
	}

	for _, value := range reflect.ValueOf(a).MapKeys() {
		v := reflect.ValueOf(a).MapIndex(value)
		fmt.Println(v.Kind().String())
	}
}

func ReadDirectory(path string) ([]string, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	ls := make([]string, len(files))
	for i, file := range files {
		ls[i] = file.Name()
	}

	return ls, nil
}

func Glob(glob string) ([]string, error) {
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}

	return files, nil
}

func ReadFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if fi.Size() > 10*1024*1024 {
		return nil, fmt.Errorf("File larger than 10MiB.")
	}

	buf := make([]byte, fi.Size())
	if _, err := io.ReadFull(file, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

func WriteFile(filename string, data []byte, DoNotOverwrite, Append bool) error {
	flags := os.O_RDWR | os.O_CREATE
	if DoNotOverwrite {
		flags |= os.O_EXCL
	}

	if !Append {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}

	file, err := os.OpenFile(filename, flags, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	return nil
}

var suffixRegexp = regexp.MustCompile(`.((_\d+)?)(\.\w*)?$`)

func EnsureNonexistentPath(name string) (string, error) {
	index := 1
	for {
		_, err := os.Stat(name)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return "", err
		}

		loc := suffixRegexp.FindStringSubmatchIndex(name)
		name = name[:loc[2]] + "_" + strconv.Itoa(index) + name[loc[3]:]
		index++
	}

	return name, nil
}

func GetInfo(path string) (*FileEntry, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("file does not exist")
		}
		return nil, err
	}

	return &FileEntry{
		Name:     fi.Name(),
		FullPath: path,
		IsDir:    fi.IsDir(),
		Size:     fi.Size(),
		Mode:     fi.Mode(),
		Time:     fi.ModTime(),
	}, nil
}

type FileEntry struct {
	Name     string      `json:"name"`
	FullPath string      `json:"fullPath"`
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"`
	Mode     os.FileMode `json:"mode"`
	Time     time.Time   `json:"time"`
}

func SetPermissions(name string, mode os.FileMode, recursive bool) error {
	var doChange func(name string) error

	doChange = func(name string) error {
		if err := os.Chmod(name, mode); err != nil {
			return err
		}

		if !recursive {
			return nil
		}

		fi, err := os.Stat(name)
		if err != nil {
			return err
		}

		if !fi.IsDir() {
			return nil
		}

		dir, err := os.Open(name)
		if err != nil {
			return err
		}
		defer dir.Close()

		entries, err := dir.Readdirnames(0)
		if err != nil {
			return err
		}
		var firstErr error
		for _, entry := range entries {
			err := doChange(name + "/" + entry)
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}

	return doChange(name)
}
