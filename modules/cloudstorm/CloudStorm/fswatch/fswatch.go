// -------------------- fswatch/fswatch.go --------------------

// supports proof of state protocol, dynamically watches own file structure
package fswatch

import (
	trinity "CloudStorm/trinitygo"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

func WatchForUpdates(basedir string, updateChan chan<- string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("Watcher error:", err)
		return
	}
	defer watcher.Close()
	err = filepath.Walk(basedir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		log.Println("Error walking basedir:", err)
		return
	}
	log.Println("Watching", basedir, "for changes...")
	throttle := time.NewTicker(1 * time.Second)
	defer throttle.Stop()
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				<-throttle.C
				newSID, err := trinity.ComputeServiceID(basedir)
				if err != nil {
					log.Println("ComputeServiceID error:", err)
					continue
				}
				updateChan <- newSID
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Watcher error:", err)
		}
	}
}
