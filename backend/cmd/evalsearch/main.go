// Command evalsearch scores plain-language search against a running server
// whose teams hold the eval photos. Each team is given by its template key,
// a token, and the photos file from uploading (path, photo id, date).
//
//	evalsearch -server http://127.0.0.1:8080 -queries eval/search/queries.tsv \
//	    -team construction_renovation=TOKEN,uploaded.tsv -team hospital_orthopedics=TOKEN,uploaded.tsv
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"

	pidexv1 "pidex/gen/pidex/v1"
	"pidex/gen/pidex/v1/pidexv1connect"
)

type photo struct {
	path string
	day  time.Time
}

type team struct {
	client pidexv1connect.SearchServiceClient
	photos map[string]photo // by photo id
}

type teamsFlag map[string]string

func (t teamsFlag) String() string { return fmt.Sprint(map[string]string(t)) }
func (t teamsFlag) Set(v string) error {
	k, rest, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("want template=TOKEN,photos.tsv")
	}
	t[k] = rest
	return nil
}

func main() {
	server := flag.String("server", "http://127.0.0.1:8080", "API base URL")
	queries := flag.String("queries", "", "queries TSV")
	zone := flag.String("tz", "Asia/Shanghai", "time zone the requests are made in")
	specs := teamsFlag{}
	flag.Var(specs, "team", "template=TOKEN,photos.tsv (repeatable)")
	flag.Parse()
	if *queries == "" || len(specs) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	teams := map[string]*team{}
	for key, spec := range specs {
		token, file, _ := strings.Cut(spec, ",")
		photos, err := readPhotos(file)
		if err != nil {
			fail(err)
		}
		auth := connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+token)
				return next(ctx, req)
			}
		}))
		teams[key] = &team{client: pidexv1connect.NewSearchServiceClient(http.DefaultClient, *server, auth), photos: photos}
	}

	f, err := os.Open(*queries)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	loc, _ := time.LoadLocation(*zone)
	today := time.Now().In(loc)
	var recalls []float64
	var wrong, passed, total, returned, returnedRelevant int
	var latencies []time.Duration
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		t := teams[cols[0]]
		if t == nil {
			continue
		}
		total++
		start := time.Now()
		res, err := t.client.SearchPhotos(context.Background(), connect.NewRequest(&pidexv1.SearchPhotosRequest{Query: cols[1], TimeZone: *zone}))
		latency := time.Since(start)
		latencies = append(latencies, latency)
		if err != nil {
			fmt.Printf("ERR  %-28s %v\n", cols[1], err)
			continue
		}
		m := res.Msg
		chips := describe(m.Filters)
		switch want := cols[2]; {
		case want == "notfound":
			ok := m.SubjectNotFound
			report(ok, cols[1], fmt.Sprintf("subject not found: %v, %d photos by content", m.SubjectNotFound, len(m.Photos)), chips, latency)
			if ok {
				passed++
			}
		case want == "none":
			ok := len(m.Photos) == 0
			report(ok, cols[1], fmt.Sprintf("%d photos", len(m.Photos)), chips, latency)
			if ok {
				passed++
			}
		case strings.HasPrefix(want, "subject:"):
			name := strings.TrimPrefix(want, "subject:")
			var subject string
			for _, sub := range m.Subjects {
				if slices.Contains(m.Filters.SubjectIds, sub.Id) && strings.Contains(sub.DisplayName, name) {
					subject = sub.Id
				}
			}
			stray := 0
			for _, ph := range m.Photos {
				if ph.SubjectId != subject {
					stray++
				}
			}
			ok := subject != "" && len(m.Photos) > 0 && stray == 0
			report(ok, cols[1], fmt.Sprintf("%d photos of %q, %d filed elsewhere", len(m.Photos)-stray, name, stray), chips, latency)
			if ok {
				passed++
			}
		case strings.HasPrefix(want, "missing:"):
			var names []string
			for _, s := range m.Missing {
				names = append(names, s.DisplayName)
			}
			ok := m.Filters.Mode == pidexv1.SearchMode_SEARCH_MODE_MISSING
			for _, w := range strings.Split(strings.TrimPrefix(want, "missing:"), ",") {
				ok = ok && slices.ContainsFunc(names, func(n string) bool { return strings.Contains(n, w) })
			}
			report(ok, cols[1], fmt.Sprintf("missing %v", names), chips, latency)
			if ok {
				passed++
			}
		default:
			frags, days := want, 0
			if i := strings.Index(want, " @days:"); i >= 0 {
				frags = want[:i]
				days, _ = strconv.Atoi(want[i+7:])
			}
			fs := strings.Split(frags, ",")
			match := func(p photo) bool {
				if !slices.ContainsFunc(fs, func(f string) bool { return strings.Contains(p.path, f) }) {
					return false
				}
				return days == 0 || !p.day.Before(today.AddDate(0, 0, -days).Truncate(24*time.Hour))
			}
			relevant := 0
			for _, p := range t.photos {
				if match(p) {
					relevant++
				}
			}
			// Judge as many top results as there are relevant photos.
			k := max(relevant, 1)
			found, bad := 0, []string{}
			for i, ph := range m.Photos {
				if i >= k {
					break
				}
				if match(t.photos[ph.Id]) {
					found++
				} else {
					bad = append(bad, short(t.photos[ph.Id].path))
				}
			}
			for _, ph := range m.Photos {
				returned++
				if match(t.photos[ph.Id]) {
					returnedRelevant++
				}
			}
			recall := 1.0
			if relevant > 0 {
				recall = float64(found) / float64(relevant)
			}
			recalls = append(recalls, recall)
			wrong += len(bad)
			ok := recall >= 0.99 && len(bad) == 0
			if ok {
				passed++
			}
			report(ok, cols[1], fmt.Sprintf("found %d/%d in top %d, %d wrong %v, %d returned", found, relevant, k, len(bad), bad, len(m.Photos)), chips, latency)
		}
	}
	slices.Sort(latencies)
	avg := 0.0
	for _, r := range recalls {
		avg += r
	}
	fmt.Printf("\n%d/%d requests fully right; mean recall %.2f; %d wrong photos in top results; %d of %d photos returned are relevant (%.0f%%); latency p50 %v p90 %v\n",
		passed, total, avg/float64(max(1, len(recalls))), wrong, returnedRelevant, returned, 100*float64(returnedRelevant)/float64(max(1, returned)),
		latencies[len(latencies)/2].Round(10*time.Millisecond), latencies[len(latencies)*9/10].Round(10*time.Millisecond))
}

func report(ok bool, query, result, chips string, latency time.Duration) {
	mark := "OK  "
	if !ok {
		mark = "MISS"
	}
	fmt.Printf("%s %-26s %-60s [%s] %v\n", mark, query, result, chips, latency.Round(10*time.Millisecond))
}

func describe(f *pidexv1.SearchFilters) string {
	var parts []string
	if f.Mode == pidexv1.SearchMode_SEARCH_MODE_MISSING {
		parts = append(parts, "MISSING")
	}
	if f.SubjectQuery != "" {
		parts = append(parts, "subject="+f.SubjectQuery)
	}
	if f.SubjectType != "" {
		parts = append(parts, "type="+f.SubjectType)
	}
	if len(f.Tags) > 0 {
		parts = append(parts, "tags="+strings.Join(f.Tags, "+"))
	}
	if f.From != nil || f.To != nil {
		from, to := "", ""
		if f.From != nil {
			from = f.From.AsTime().Format("01-02")
		}
		if f.To != nil {
			to = f.To.AsTime().Format("01-02")
		}
		parts = append(parts, "date="+from+".."+to)
	}
	if f.Text != "" {
		parts = append(parts, "text="+f.Text)
	}
	return strings.Join(parts, " ")
}

func short(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	if len(p) > 24 {
		p = p[:24]
	}
	return p
}

func readPhotos(path string) (map[string]photo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]photo{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) < 3 {
			continue
		}
		day, err := time.Parse("2006-01-02", cols[2])
		if err != nil {
			return nil, err
		}
		out[cols[1]] = photo{path: cols[0], day: day}
	}
	return out, sc.Err()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
