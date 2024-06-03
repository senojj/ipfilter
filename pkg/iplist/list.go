package iplist

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"net/http"
)

// maxDownloadBytes is a defensive measure to prevent a malicious
// man-in-the-middle from causing memory exhaustion in this service.
const maxDownloadBytes int = 50_000_000 // 50MB

var UnchangedVersion = errors.New("version unchanged")

// parseAddress turns a string network address into a full CIDR
// and parses the resulting CIDR into a net.IPNet value. Any
// address provided without a subnet mask will be assumed as
// a /32 network.
func parseAddress(address string) (*net.IPNet, error) {
	if !strings.Contains(address, "/") {
		address += "/32"
	}
	_, n, err := net.ParseCIDR(address)
	return n, err
}

// List contains a set of bad address CIDRs. This data structure is
// thread safe and allows multiple reads at once. List makes no attempt
// to shrink the underlying array when values are no longer included in
// the set.
type List struct {
	lock   sync.RWMutex
	values []*net.IPNet

	// Version indicates the current version of the list data.
	Version string

	// LastRefresh indicates the last time that an attempt was
	// made to refresh the list data.
	LastRefresh time.Time
}

// NewList creates a new bad ip *List and sets its internal array capacity
// to the given size value.
func NewList(size int) *List {
	return &List{
		values: make([]*net.IPNet, size),
	}
}

// Len returns the number of entries in the list. If a nil value is
// encountered, the function will return before traversing the list
// in its entirety.
func (l *List) Len() (i int) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	for ; i < len(l.values); i++ {
		if l.values[i] == nil {
			return
		}
	}
	return
}

// Contains returns true when the given ip address exists within
// any one of the CIDRs contained within the list of bad addresses.
// This check will traverse the entire list of bad addresses until
// a match is found, or a nil value is encountered. If a nil value
// is encountered, all remaining indexes should also be nil, so it
// is favorable to return early. This data structure never shrinks
// the underlying array, to save compute cycles. A read lock is
// obtained before traversing the bad addresses.
func (l *List) Contains(ip net.IP) bool {
	if ip == nil {
		return false
	}
	l.lock.RLock()
	defer l.lock.RUnlock()
	for _, n := range l.values {
		if n == nil {
			return false
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Add replaces any current bad addresses in the list with a new
// set of bad addresses. If the new set of bad addresses is smaller
// than the existing set, any indexes above the largest index of
// new set are assigned a nil value so that the old values may be
// collected by the garbage collector. A write lock is obtained
// before replacing the current set of addresses.
func (l *List) Add(addresses []*net.IPNet) {
	l.lock.Lock()
	defer l.lock.Unlock()
	var i int
	for ; i < len(addresses); i++ {
		l.values[i] = addresses[i]
	}
	for ; i < len(l.values); i++ {
		l.values[i] = nil
	}
}

// GitHubLoader loads bad IP address lists from the firehol/blocklist-ipsets
// GitHub repository, specifically. The entire master branch of the repo is
// downloaded as an archive file and processed into a List. Only files in
// the archive whose name suffix matches the values in fileSuffixList will
// be processed.
type GitHubLoader struct {
	archiveURL     string
	fileSuffixList []string
	logger         *slog.Logger
}

// NewGitHubLoader returns a newly instantiated GitHubLoader with the provided
// configuration parameters.
func NewGitHubLoader(archiveURL string, fileSuffixList []string, logger *slog.Logger) *GitHubLoader {
	return &GitHubLoader{
		archiveURL:     archiveURL,
		fileSuffixList: fileSuffixList,
		logger:         logger,
	}
}

// Load will attempt to refresh the entries in the List. First a HEAD request
// will be made to the repository. If the returned ETag is different from the
// last seen value, or if this is the first time the List is being refreshed,
// Load will make a GET request to the repository to download a zip archive
// of the entire master branch. All addresses contained within the archive files
// are parsed into valid net.IPNet values before being added to the List.
//
// The found value indicates the number of valid values that were identified in
// the archive. If the found value is greater than the length of the List, then
// the capacity of the List was not sufficient to hold all found values.
func (l *GitHubLoader) Load(list *List) (found int, err error) {
	// Record an attempt to refresh the list.
	list.LastRefresh = time.Now()

	// The version of the resource may not have changed since the last
	// download, so before requesting the resource data, the header is
	// requested to compare the version.
	resp, err := http.Head(l.archiveURL)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
	tETag := resp.Header.Get("ETag")

	if tETag == list.Version {
		err = UnchangedVersion
		return
	}

	// The version of the resource is different, so the data needs to be
	// refreshed with the new version.
	resp, err = http.Get(l.archiveURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	// Allocate an initial amount of space to hold the downloaded
	// data. This will mitigate growth operations of the backing
	// array.
	buf := bytes.NewBuffer(make([]byte, 0, maxDownloadBytes))

	// Since the response body has a transfer encoding of "chunked"
	// we will not know the size of the payload before reading to
	// EOF. Therefore, io.Copy is not a safe choice to use here, as
	// a malicious downstream server could send an unbounded payload.
	// Instead, calls to Read will be made iteratively, 1024 bytes at
	// time, up to maxDownloadBytes.
	ibuf := make([]byte, 1024)
	for i := 0; i < maxDownloadBytes; {
		var bread int
		bread, err = resp.Body.Read(ibuf)
		if err == io.EOF && bread == 0 {
			break
		}
		if err != nil {
			return
		}
		buf.Write(ibuf[:bread])
		i += bread
	}
	copied := int64(buf.Len())

	// An error resulting from closing the body may be indicative
	// of an issue with the response payload.
	err = resp.Body.Close()
	if err != nil {
		return
	}
	reader := bytes.NewReader(buf.Bytes())
	zipReader, err := zip.NewReader(reader, copied)
	if err != nil {
		return
	}

	// Here a trade-off is made by using additional memory to preserve the
	// integrity of the current bad ip list. Making this trade-off also reduces
	// the amount of time that a write lock will be held on the list.
	// An alternative would be to write directly to the list.values array,
	// however, an error during parsing could leave the list in a broken state.
	results := make([]chan *net.IPNet, 0, len(zipReader.File))
	for _, file := range zipReader.File {
		if file.FileHeader.FileInfo().IsDir() {
			continue
		}

		var processFile bool
		for i := 0; i < len(l.fileSuffixList); i++ {
			if strings.HasSuffix(file.Name, l.fileSuffixList[i]) {
				processFile = true
			}
		}

		if !processFile {
			continue
		}

		var f io.ReadCloser
		f, err = file.Open()
		if err != nil {
			l.logger.Warn(fmt.Sprintf("open file: %e", err), "file", file.Name)
			continue
		}
		ch := make(chan *net.IPNet, 100)
		go func() {
			defer close(ch)
			defer f.Close()

			scn := bufio.NewScanner(f)
			for scn.Scan() {
				line := scn.Text()
				if strings.HasPrefix(line, "#") {
					continue
				}
				var addr *net.IPNet
				addr, err = parseAddress(strings.TrimSpace(line))
				if err != nil {
					l.logger.Warn(fmt.Sprintf("parse address: %e", err), "address", line)
					continue
				}
				ch <- addr
			}
		}()
		results = append(results, ch)
	}
	collection := make([]*net.IPNet, 0, cap(list.values))

	for {
		var alive bool

		// iterate over channels and pull out the next available item, but
		// don't wait for an item to become available.
		for i := 0; i < len(results); i++ {
			select {
			case v, ok := <-results[i]:
				if ok {
					collection = append(collection, v)
					alive = true
				}
			default:
				// the channel is still open, but there weren't any items
				// waiting to be processed.
				alive = true
			}
		}
		if !alive {
			// all channels have been closed at this point
			break
		}
	}
	found = len(collection)
	list.Add(collection)
	list.Version = tETag
	return
}
