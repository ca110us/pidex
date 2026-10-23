// Command evalembed compares text embedding models for photo search. The
// documents are what search indexes for each photo (the vision model's
// description and tags, from an evalvision -out file); the queries list
// the photos they should find. Nothing here knows an industry: -own says
// which team config each photo belongs to, by path prefix.
//
//	evalembed -answers last-run.jsonl -queries queries.tsv \
//	    -own ortho=hospital_orthopedics -own construction=construction_renovation -own junk=construction_renovation \
//	    model[:dims]...
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"pidex/internal/ai"
	"pidex/internal/search"
	"pidex/internal/teamconfig"
)

var showGaps bool

// ownFlag maps a path prefix to the template its photos were labeled for.
type ownFlag map[string]string

func (o ownFlag) String() string { return fmt.Sprint(map[string]string(o)) }
func (o ownFlag) Set(v string) error {
	prefix, tpl, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("want prefix=template")
	}
	o[prefix] = tpl
	return nil
}

// owner is the template of the longest matching prefix, or "".
func (o ownFlag) owner(path string) string {
	best, tpl := -1, ""
	for prefix, t := range o {
		if strings.HasPrefix(path, prefix) && len(prefix) > best {
			best, tpl = len(prefix), t
		}
	}
	return tpl
}

type query struct {
	text     string
	relevant []string
}

func main() {
	answers := flag.String("answers", "", "evalvision -out file")
	queries := flag.String("queries", "", "queries TSV")
	own := ownFlag{}
	flag.Var(own, "own", "prefix=template: photos under this path prefix are indexed with that team config (repeatable)")
	gaps := flag.Bool("gaps", false, "per query, print the similarity of the weakest relevant and strongest irrelevant photo (to choose a cutoff)")
	flag.Parse()
	showGaps = *gaps
	key := os.Getenv("OPENROUTER_API_KEY")
	if *answers == "" || *queries == "" || key == "" || len(own) == 0 || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: OPENROUTER_API_KEY=... evalembed -answers FILE -queries FILE -own prefix=template... model[:dims]...")
		os.Exit(2)
	}
	paths, docs, err := readDocs(*answers, own)
	if err != nil {
		fail(err)
	}
	qs, err := readQueries(*queries)
	if err != nil {
		fail(err)
	}
	fmt.Printf("%d photos, %d queries\n", len(paths), len(qs))
	for _, spec := range flag.Args() {
		model, dims := spec, 0
		if i := strings.LastIndex(spec, ":"); i > 0 {
			if n, err := strconv.Atoi(spec[i+1:]); err == nil {
				model, dims = spec[:i], n
			}
		}
		e := &ai.OpenAICompatible{Name: "openrouter", BaseURL: ai.OpenRouterBaseURL, APIKey: key, ModelID: model, EmbeddingDimensions: dims}
		if err := run(e, spec, paths, docs, qs); err != nil {
			fmt.Printf("%-40s ERROR %v\n", spec, err)
		}
	}
}

func run(e *ai.OpenAICompatible, spec string, paths, docs []string, qs []query) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	start := time.Now()
	dv, u1, err := e.EmbedTexts(ctx, docs)
	if err != nil {
		return err
	}
	texts := make([]string, len(qs))
	for i, q := range qs {
		texts[i] = q.text
	}
	qv, u2, err := e.EmbedTexts(ctx, texts)
	if err != nil {
		return err
	}
	recall := map[string][]float64{}
	mrr := map[string][]float64{}
	for i, q := range qs {
		type hit struct {
			score float64
			path  string
		}
		hits := make([]hit, len(paths))
		for j := range paths {
			hits[j] = hit{cosine(qv[i], dv[j]), paths[j]}
		}
		sort.Slice(hits, func(a, b int) bool { return hits[a].score > hits[b].score })
		isRelevant := func(p string) bool {
			return slices.ContainsFunc(q.relevant, func(f string) bool { return strings.Contains(p, f) })
		}
		n := 0
		for _, p := range paths {
			if isRelevant(p) {
				n++
			}
		}
		if showGaps {
			worstRel, bestIrr := 1.0, -1.0
			for _, h := range hits {
				if isRelevant(h.path) {
					worstRel = min(worstRel, h.score)
				} else {
					bestIrr = max(bestIrr, h.score)
				}
			}
			fmt.Printf("    %-34s weakest relevant %.3f  strongest irrelevant %.3f\n", q.text, worstRel, bestIrr)
		}
		found, first := 0, 0
		for rank, h := range hits {
			if isRelevant(h.path) {
				if rank < 5 {
					found++
				}
				if first == 0 {
					first = rank + 1
				}
			}
		}
		lang := "en"
		if strings.IndexFunc(q.text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
			lang = "zh"
		}
		recall[lang] = append(recall[lang], float64(found)/float64(min(5, n)))
		mrr[lang] = append(mrr[lang], 1/float64(first))
	}
	fmt.Printf("%-40s dims=%-5d recall@5 en=%.2f zh=%.2f  MRR en=%.2f zh=%.2f  %.1fs  $%.6f\n",
		spec, len(dv[0]), avg(recall["en"]), avg(recall["zh"]), avg(mrr["en"]), avg(mrr["zh"]),
		time.Since(start).Seconds(), u1.CostUSD+u2.CostUSD)
	return nil
}

// readDocs builds each photo's search document the way the pipeline does,
// from the answers given under the photo's own team config.
func readDocs(path string, own ownFlag) ([]string, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	configs := map[string]*teamconfig.Config{}
	byPath := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var rec struct {
			Path     string `json:"path"`
			Template string `json:"template"`
			Result   *struct {
				Description string `json:"description"`
				Tags        []struct {
					Key string `json:"key"`
				} `json:"tags"`
			} `json:"result"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil || rec.Result == nil {
			continue
		}
		tpl := own.owner(rec.Path)
		if tpl == "" || rec.Template != tpl {
			continue
		}
		cfg := configs[tpl]
		if cfg == nil {
			if cfg, err = teamconfig.LoadTemplate(tpl); err != nil {
				return nil, nil, err
			}
			configs[tpl] = cfg
		}
		var tags []string
		for _, t := range rec.Result.Tags {
			tags = append(tags, t.Key)
		}
		byPath[rec.Path] = search.Document(cfg, search.DocInput{Description: rec.Result.Description, TagKeys: tags})
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	docs := make([]string, len(paths))
	for i, p := range paths {
		docs[i] = byPath[p]
	}
	return paths, docs, sc.Err()
}

func readQueries(path string) ([]query, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var qs []query
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		text, rel, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, fmt.Errorf("bad line %q", line)
		}
		qs = append(qs, query{text: text, relevant: strings.Split(rel, ",")})
	}
	return qs, sc.Err()
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / (math.Sqrt(na)*math.Sqrt(nb) + 1e-12)
}

func avg(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(max(1, len(xs)))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
