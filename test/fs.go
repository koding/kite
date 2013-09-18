package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/howeyc/fsnotify"
	"io"
	"io/ioutil"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"koding/tools/dnode"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

type Os struct{}

var (
	port = flag.String("port", "", "port to bind itself")

	// watcher variables
	once               sync.Once
	newPaths, oldPaths = make(chan string), make(chan string)
	watchCallbacks     = make(map[string]func(*fsnotify.FileEvent), 100) // Limit of watching folders
)

func main() {
	flag.Parse()
	o := &protocol.Options{Username: "fatih", Kitename: "fs-local", Version: "1", Port: *port}
	k := kite.New(o, new(Os))
	k.Start()
}

func (Os) ReadDirectory(r *protocol.KiteDnodeRequest, result *map[string]interface{}) error {
	var params struct {
		Path                string
		OnChange            dnode.Callback
		WatchSubdirectories bool
	}

	if r.Args.Unmarshal(&params) != nil || params.Path == "" {
		return errors.New("{ path: [string], onChange: [function], watchSubdirectories: [bool] }")
	}

	response := make(map[string]interface{})

	if params.OnChange != nil {
		onceBody := func() { startWatcher() }
		go once.Do(onceBody)

		// notify new paths to the watcher
		newPaths <- params.Path

		var event string
		var fileEntry *FileEntry
		changer := func(ev *fsnotify.FileEvent) {
			if ev.IsCreate() {
				event = "added"
				fileEntry, _ = GetInfo(ev.Name)
			} else if ev.IsDelete() {
				event = "removed"
				fileEntry = &FileEntry{Name: path.Base(ev.Name), FullPath: ev.Name}
			}

			params.OnChange(map[string]interface{}{
				"event": event,
				"file":  fileEntry,
			})
			return
		}

		watchCallbacks[params.Path] = changer

		// this callback is called whenever we receive a 'stopWatching' from the client
		response["stopWatching"] = func() {
			delete(watchCallbacks, params.Path)
			oldPaths <- params.Path
		}
	}

	files, err := ReadDirectory(params.Path)
	if err != nil {
		return err
	}

	response["files"] = files
	*result = response
	return nil
}

func (Os) Glob(r *protocol.KiteDnodeRequest, result *[]string) error {
	var params struct {
		Pattern string
	}

	if r.Args.Unmarshal(&params) != nil || params.Pattern == "" {
		return errors.New("{ pattern: [string] }")
	}

	files, err := Glob(params.Pattern)
	if err != nil {
		return err
	}

	*result = files
	return nil
}

func (Os) ReadFile(r *protocol.KiteDnodeRequest, result *map[string]interface{}) error {
	var params struct {
		Path string
	}
	if r.Args.Unmarshal(&params) != nil || params.Path == "" {
		return errors.New("{ path: [string] }")
	}

	buf, err := ReadFile(params.Path)
	if err != nil {
		return err
	}

	*result = map[string]interface{}{"content": buf}
	return nil
}

func (Os) WriteFile(r *protocol.KiteDnodeRequest, result *string) error {
	var params struct {
		Path           string
		Content        []byte
		DoNotOverwrite bool
		Append         bool
	}

	if r.Args.Unmarshal(&params) != nil || params.Path == "" || params.Content == nil {
		return errors.New("{ path: [string], content: [base64], doNotOverwrite: [bool], append: [bool] }")
	}

	err := WriteFile(params.Path, params.Content, params.DoNotOverwrite, params.Append)
	if err != nil {
		return err
	}

	*result = fmt.Sprintf("content written to %s", params.Path)
	return nil
}

func (Os) EnsureNonexistentPath(r *protocol.KiteDnodeRequest, result *string) error {
	var params struct {
		Path string
	}

	if r.Args.Unmarshal(&params) != nil || params.Path == "" {
		return errors.New("{ path: [string] }")
	}

	name, err := EnsureNonexistentPath(params.Path)
	if err != nil {
		return err
	}

	*result = name
	return nil
}

func (Os) GetInfo(r *protocol.KiteDnodeRequest, result *FileEntry) error {
	var params struct {
		Path string
	}
	if r.Args.Unmarshal(&params) != nil || params.Path == "" {
		return errors.New("{ path: [string] }")
	}

	fileEntry, err := GetInfo(params.Path)
	if err != nil {
		return err
	}

	*result = *fileEntry
	return nil
}

func (Os) SetPermissions(r *protocol.KiteDnodeRequest, result *bool) error {
	var params struct {
		Path      string
		Mode      os.FileMode
		Recursive bool
	}
	if r.Args.Unmarshal(&params) != nil || params.Path == "" {
		return errors.New("{ path: [string], mode: [integer], recursive: [bool] }")
	}

	err := SetPermissions(params.Path, params.Mode, params.Recursive)
	if err != nil {
		return err
	}

	*result = true
	return nil

}

func (Os) Remove(r *protocol.KiteDnodeRequest, result *bool) error {
	var params struct {
		Path      string
		Recursive bool
	}

	if r.Args.Unmarshal(&params) != nil || params.Path == "" {
		return errors.New("{ path: [string], recursive: [bool] }")
	}

	err := Remove(params.Path)
	if err != nil {
		return err
	}

	*result = true
	return nil
}

func (Os) Rename(r *protocol.KiteDnodeRequest, result *bool) error {
	var params struct {
		OldPath string
		NewPath string
	}

	if r.Args.Unmarshal(&params) != nil || params.OldPath == "" || params.NewPath == "" {
		return errors.New("{ oldPath: [string], newPath: [string] }")
	}

	err := Rename(params.OldPath, params.NewPath)
	if err != nil {
		return err
	}

	*result = true
	return nil
}

func (Os) CreateDirectory(r *protocol.KiteDnodeRequest, result *bool) error {
	var params struct {
		Path      string
		Recursive bool
	}
	if r.Args.Unmarshal(&params) != nil || params.Path == "" {
		return errors.New("{ path: [string], recursive: [bool] }")
	}

	err := CreateDirectory(params.Path, params.Recursive)
	if err != nil {
		return err
	}
	*result = true
	return nil
}

/****************************************
*
* Move the functions below to a seperate package
*
*****************************************/
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

func startWatcher() {
	var err error
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case p := <-newPaths:
				err := watcher.Watch(p)
				if err != nil {
					log.Println("watch path adding", err)
				}
			case p := <-oldPaths:
				err := watcher.RemoveWatch(p)
				if err != nil {
					log.Println("watch remove adding", err)
				}
			}
		}
	}()

	for event := range watcher.Event {
		f, ok := watchCallbacks[path.Dir(event.Name)]
		if !ok {
			continue
		}

		f(event)
	}
}
