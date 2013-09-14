package main

import (
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"io"
	"io/ioutil"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"log"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"

	"time"
)

type Os struct{}

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()
	o := &protocol.Options{Username: "fatih", Kitename: "os-local", Version: "1", Port: *port}
	k := kite.New(o, new(Os))

	go func() {
		var event string
		for change := range watcher() {
			if change.IsCreate() {
				event = "added"
			} else if change.IsDelete() {
				event = "removed"
			} else {
				continue
			}

			fileEntry := FileEntry{Name: path.Base(change.Name), FullPath: change.Name}
			fmt.Println("changed", change.Name)

			msg := struct {
				Event string    `json:"event"`
				File  FileEntry `json:"file"`
			}{
				event,
				fileEntry,
			}

			k.SendMsg("devrim", "onChange", msg)
		}
	}()

	k.Start()
}

func (Os) ReadDirectory(r *protocol.KiteRequest, result *map[string]interface{}) error {

	fmt.Println(r.Username, r.Kitename, r.Origin, r.Method)

	params := r.Args.(map[string]interface{})
	path, ok := params["path"].(string)
	if !ok {
		return errors.New("path argument missing")
	}

	response := make(map[string]interface{})
	files, err := ReadDirectory(path)
	if err != nil {
		return err
	}

	response["files"] = files
	*result = response
	return nil
}

func (Os) Glob(r *protocol.KiteRequest, result *[]string) error {
	params := r.Args.(map[string]interface{})
	glob, ok := params["pattern"].(string)
	if !ok {
		return errors.New("pattern argument missing")
	}

	files, err := Glob(glob)
	if err != nil {
		return err
	}

	*result = files

	return nil
}

func (Os) ReadFile(r *protocol.KiteRequest, result *map[string]interface{}) error {
	params := r.Args.(map[string]interface{})
	path, ok := params["path"].(string)
	if !ok {
		return errors.New("path argument missing")
	}
	buf, err := ReadFile(path)
	if err != nil {
		return err
	}

	*result = map[string]interface{}{"content": buf}
	return nil
}

func (Os) WriteFile(r *protocol.KiteRequest, result *string) error {
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
	params := r.Args.(map[string]interface{})
	path, ok := params["path"].(string)
	if !ok {
		return errors.New("path argument missing")
	}
	name, err := EnsureNonexistentPath(path)
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

func (Os) Remove(r *protocol.KiteRequest, result *bool) error {
	params := r.Args.(map[string]interface{})
	path, ok := params["path"].(string)
	if !ok {
		return errors.New("path argument missing")
	}

	err := Remove(path)
	if err != nil {
		return err
	}

	*result = true
	return nil
}

func (Os) Rename(r *protocol.KiteRequest, result *bool) error {
	params := r.Args.(map[string]interface{})
	oldPath, ok := params["oldPath"].(string)
	if !ok {
		return errors.New("oldPath argument missing")
	}

	newPath, ok := params["newPath"].(string)
	if !ok {
		return errors.New("newPath argument missing")
	}

	err := Rename(oldPath, newPath)
	if err != nil {
		return err
	}

	*result = true
	return nil
}

func (Os) CreateDirectory(r *protocol.KiteRequest, result *bool) error {
	params := r.Args.(map[string]interface{})
	path, ok := params["path"].(string)
	if !ok {
		return errors.New("path argument missing")
	}
	recursive, ok := params["recursive"].(bool)
	if !ok {
		return errors.New("recursive argument missing")
	}

	err := CreateDirectory(path, recursive)
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

func ReadDirectory(p string) ([]FileEntry, error) {
	files, err := ioutil.ReadDir(p)
	if err != nil {
		return nil, err
	}

	ls := make([]FileEntry, len(files))
	for i, info := range files {
		ls[i] = makeFileEntry(path.Join(p, info.Name()), info)
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

	fileEntry := makeFileEntry(path, fi)

	return &fileEntry, nil
}

func makeFileEntry(fullPath string, fi os.FileInfo) FileEntry {
	entry := FileEntry{
		Name:     fi.Name(),
		FullPath: fullPath,
		IsDir:    fi.IsDir(),
		Size:     fi.Size(),
		Mode:     fi.Mode(),
		Time:     fi.ModTime(),
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		symlinkInfo, err := os.Stat(path.Dir(fullPath) + "/" + fi.Name())
		if err != nil {
			entry.IsBroken = true
			return entry
		}
		entry.IsDir = symlinkInfo.IsDir()
		entry.Size = symlinkInfo.Size()
		entry.Mode = symlinkInfo.Mode()
		entry.Time = symlinkInfo.ModTime()
	}

	return entry
}

type FileEntry struct {
	Name     string      `json:"name"`
	FullPath string      `json:"fullPath"`
	IsDir    bool        `json:"isDir"`
	Size     int64       `json:"size"`
	Mode     os.FileMode `json:"mode"`
	Time     time.Time   `json:"time"`
	IsBroken bool        `json:"isBroken"`
	Readable bool        `json:"readable"`
	Writable bool        `json:"writable"`
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

func Remove(path string) error {
	return os.Remove(path)
}

func Rename(oldname, newname string) error {
	return os.Rename(oldname, newname)
}

func CreateDirectory(name string, recursive bool) error {
	if recursive {
		return os.MkdirAll(name, 0755)
	}

	return os.Mkdir(name, 0755)
}

func watcher() chan *fsnotify.FileEvent {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	err = watcher.Watch("/Users/fatih/Code")
	if err != nil {
		log.Fatal(err)
	}

	return watcher.Event
}
