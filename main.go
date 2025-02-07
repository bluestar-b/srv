package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

func FileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	} else {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	}
}

func FileCreationDate(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

type context struct {
	srvDir string
}

const listingPrelude = `<head>
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="icon" href="data:,">
<style>

* {
     font-family: monospace;
}
 table {
     width: 100%;
}
 table {
     border-spacing: 0;
     border-collapse: collapse;
}
 td {
     padding:0px;
     overflow: hidden;
     text-overflow: ellipsis;
     white-space: nowrap;
}
 body {
     background-color: black;
     color: white;
}
 a:hover {
     color: #eeb9da
}
 a {
     color: #ff3d98
}

</style>
</head>
<table cellspacing="0">
<thead>
    <tr><th>Name</th><th>Size</th><th>Date</th></tr>
</thead>
<tbody>`

func renderListing(w http.ResponseWriter, r *http.Request, f *os.File) error {
	files, err := f.Readdir(-1)
	if err != nil {
		return err
	}

	io.WriteString(w, listingPrelude)

	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name()) < strings.ToLower(files[j].Name())
	})

	var fn, fnEscaped string
	for _, fi := range files {
		fn = fi.Name()
		fnEscaped = url.PathEscape(fn)
		creationDate := FileCreationDate(fi.ModTime())
		switch m := fi.Mode(); {
		case m&os.ModeDir != 0:
			fmt.Fprintf(w, "<tr><td><a href=\"%s/\">%s/</a></td><td></td><td></td></tr>", fnEscaped, fn)
		case m&os.ModeType == 0:
			fs := FileSize(fi.Size())
			fmt.Fprintf(w, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td></tr>", fnEscaped, fn, fs, creationDate)
		default:
			fmt.Fprintf(w, "<tr><td><p>%s</p></td><td></td><td></td></tr>", fn)
		}
	}

	io.WriteString(w, "</tbody></table>")
	return nil
}

func (c *context) handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("\t%s [%s]: %s %s %s", r.RemoteAddr, r.UserAgent(), r.Method, r.Proto, r.Host+r.RequestURI)

	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle OPTIONS request for CORS preflight
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Cache-Control", "no-store")

	switch r.Method {
	case http.MethodGet:
		fp, err := url.PathUnescape(r.RequestURI)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to path unescape: %s", err), http.StatusInternalServerError)
			return
		}
		fp = path.Join(c.srvDir, fp)
		fi, err := os.Lstat(fp)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "file not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to stat file: %s", err), http.StatusInternalServerError)
			return
		}

		f, err := os.Open(fp)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to open file: %s", err), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		switch m := fi.Mode(); {
		case m&os.ModeDir != 0:
			html, err := os.Open(path.Join(fp, "index.html"))
			if err == nil {
				io.Copy(w, html)
				html.Close()
				return
			}
			html.Close()
			err = renderListing(w, r, f)
			if err != nil {
				http.Error(w, "failed to render directory listing: "+err.Error(), http.StatusInternalServerError)
			}
		case m&os.ModeType == 0:
			http.ServeContent(w, r, fp, time.Time{}, f)
		case m&os.ModeSymlink != 0:
			http.Error(w, "file is a symlink", http.StatusForbidden)
		default:
			http.Error(w, "file isn't a regular file or directory", http.StatusForbidden)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func die(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format, v...)
	os.Stderr.Write([]byte("\n"))
	os.Exit(1)
}

var VERSION = "unknown"

func main() {
	var (
		port, bindAddr, certFile, keyFile string
		quiet                             bool
	)

	flag.BoolVar(&quiet, "q", false, "quiet; disable all logging")
	flag.StringVar(&port, "port", "8000", "port to listen on")
	flag.StringVar(&bindAddr, "bind", "127.0.0.1", "listener socket's bind address")
	flag.StringVar(&certFile, "cert", "", "path to SSL/TLS certificate file")
	flag.StringVar(&keyFile, "key", "", "path to SSL/TLS key file")
	flag.Parse()

	listenAddr := net.JoinHostPort(bindAddr, port)
	_, err := net.ResolveTCPAddr("tcp", listenAddr)
	if err != nil {
		die("Could not resolve the address to listen to: %s", listenAddr)
	}

	srvDir := "."
	posArgs := flag.Args()

	if len(posArgs) > 0 {
		srvDir = posArgs[0]
	}
	f, err := os.Open(srvDir)
	if err != nil {
		die(err.Error())
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.IsDir() {
		die("%s isn't a directory.", srvDir)
	}

	c := &context{
		srvDir: srvDir,
	}

	if quiet {
		log.SetFlags(0)
		log.SetOutput(io.Discard)
	}

	http.HandleFunc("/", c.handler)

	log.Printf("\tServing %s over HTTP on %s", srvDir, listenAddr)

	if certFile != "" && keyFile != "" {
		log.Printf("\tUsing SSL/TLS with certificate %s and key %s", certFile, keyFile)
		err = http.ListenAndServeTLS(listenAddr, certFile, keyFile, nil)
	} else {
		err = http.ListenAndServe(listenAddr, nil)
	}

	die(err.Error())
}
