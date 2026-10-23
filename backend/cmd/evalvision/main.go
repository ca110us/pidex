// Command evalvision measures how well a vision model fills in the analysis
// form for a set of labeled photos. It sends exactly what the pipeline
// sends (same instructions, schema and downscaling) and compares the answer
// with the labels.
//
// Labels are TSV, one photo per line:
//
//	path  template  roles  tags  identity
//
// roles is the accepted roles separated by "|", or "*" for any. tags lists
// tag keys that must be present, separated by ",". identity is a value that
// must appear among the identity fields, "-" if none may be reported, or
// empty to skip the check. Lines starting with "#" are comments.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"pidex/internal/ai"
	"pidex/internal/analysis"
	"pidex/internal/identity"
	"pidex/internal/imaging"
	"pidex/internal/teamconfig"
)

type label struct {
	Path     string
	Template string
	Roles    []string
	Tags     []string
	Identity string
}

type outcome struct {
	Label    label
	Result   *analysis.Result
	Usage    ai.Usage
	Err      error
	RoleOK   bool
	TagsOK   bool
	IDOK     bool
	Problems []string
}

func main() {
	labelsPath := flag.String("labels", "", "labels TSV")
	dir := flag.String("images", ".", "directory the label paths are relative to")
	model := flag.String("model", "google/gemini-2.5-flash-lite", "OpenRouter model ID")
	workers := flag.Int("workers", 4, "concurrent requests")
	maxEdge := flag.Int("max-edge", 1280, "long edge sent to the model, as in the pipeline")
	out := flag.String("out", "", "write every answer as JSON lines to this file")
	flag.Parse()

	key := os.Getenv("OPENROUTER_API_KEY")
	if *labelsPath == "" || key == "" {
		fmt.Fprintln(os.Stderr, "usage: OPENROUTER_API_KEY=... evalvision -labels labels.tsv -images DIR [-model ID]")
		os.Exit(2)
	}
	labels, err := readLabels(*labelsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	configs := map[string]*teamconfig.Config{}
	for _, l := range labels {
		if configs[l.Template] == nil {
			if configs[l.Template], err = teamconfig.LoadTemplate(l.Template); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}

	vm := &ai.OpenAICompatible{Name: "openrouter", BaseURL: ai.OpenRouterBaseURL, APIKey: key, ModelID: *model,
		ReportCost: true, Headers: map[string]string{"X-Title": "Pidex eval"}}
	router := ai.NewRouter(nil, slog.New(slog.DiscardHandler), vm)

	results := make([]outcome, len(labels))
	var wg sync.WaitGroup
	sem := make(chan struct{}, *workers)
	for i, l := range labels {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			results[i] = evaluate(router, configs[l.Template], filepath.Join(*dir, l.Path), l, *maxEdge)
			fmt.Fprint(os.Stderr, ".")
		}()
	}
	wg.Wait()
	fmt.Fprintln(os.Stderr)

	if *out != "" {
		if err := writeAnswers(*out, results); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	report(results, *model)
}

func evaluate(r *ai.Router, cfg *teamconfig.Config, path string, l label, maxEdge int) outcome {
	o := outcome{Label: l}
	data, err := os.ReadFile(path)
	if err == nil {
		data, err = imaging.Fit(data, maxEdge)
	}
	if err != nil {
		o.Err = err
		return o
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	raw, usage, err := r.Structured(ctx, ai.CallMeta{Purpose: "eval"}, ai.StructuredRequest{
		Instructions:    analysis.Instructions(cfg),
		Prompt:          analysis.Prompt,
		Images:          []ai.Image{{Data: data, MIMEType: "image/jpeg"}},
		Schema:          analysis.Schema(cfg),
		MaxOutputTokens: 1024,
	})
	o.Usage = usage
	if err == nil {
		o.Result, err = analysis.Parse(raw, cfg)
	}
	if err != nil {
		o.Err = err
		return o
	}
	res := o.Result

	o.RoleOK = slices.Contains(l.Roles, "*") || slices.Contains(l.Roles, string(res.Role))
	if !o.RoleOK {
		o.Problems = append(o.Problems, fmt.Sprintf("role %s, want %s", res.Role, strings.Join(l.Roles, "|")))
	}
	got := map[string]bool{}
	for _, t := range res.Tags {
		got[t.Key] = true
	}
	o.TagsOK = true
	for _, t := range l.Tags {
		if !got[t] {
			o.TagsOK = false
			o.Problems = append(o.Problems, "missing tag "+t)
		}
	}
	// Judge only what survives the same cleaning the pipeline applies.
	fields := map[string]string{}
	if st, ok := cfg.SubjectType(res.SubjectType); ok {
		read, _ := res.IdentityFields()
		fields, _ = identity.Clean(st, read, cfg.DateOrder)
	}
	o.IDOK = true
	switch l.Identity {
	case "":
	case "-":
		if len(fields) > 0 {
			o.IDOK = false
			o.Problems = append(o.Problems, fmt.Sprintf("invented identity %v", fields))
		}
	default:
		found := false
		for _, v := range fields {
			found = found || strings.Contains(strings.ToUpper(v), strings.ToUpper(l.Identity))
		}
		if !found {
			o.IDOK = false
			o.Problems = append(o.Problems, fmt.Sprintf("identity %v, want %q", fields, l.Identity))
		}
	}
	return o
}

func report(results []outcome, model string) {
	byTemplate := map[string][]outcome{}
	for _, o := range results {
		byTemplate[o.Label.Template] = append(byTemplate[o.Label.Template], o)
	}
	var cost float64
	var latencies []time.Duration
	for _, o := range results {
		cost += o.Usage.CostUSD
		if o.Err == nil {
			latencies = append(latencies, o.Usage.Latency)
		}
	}
	slices.Sort(latencies)

	fmt.Printf("model %s, %d photos, cost $%.4f ($%.2f per 1,000)", model, len(results), cost, cost/float64(len(results))*1000)
	if len(latencies) > 0 {
		fmt.Printf(", latency p50 %v p90 %v", latencies[len(latencies)/2].Round(10*time.Millisecond),
			latencies[len(latencies)*9/10].Round(10*time.Millisecond))
	}
	fmt.Println()

	templates := make([]string, 0, len(byTemplate))
	for t := range byTemplate {
		templates = append(templates, t)
	}
	sort.Strings(templates)
	for _, t := range templates {
		items := byTemplate[t]
		var roleN, roleOK, tagN, tagOK, idN, idOK, errs int
		confusion := map[string]int{}
		for _, o := range items {
			if o.Err != nil {
				errs++
				continue
			}
			if !slices.Contains(o.Label.Roles, "*") {
				roleN++
				if o.RoleOK {
					roleOK++
				} else {
					confusion[strings.Join(o.Label.Roles, "|")+" -> "+string(o.Result.Role)]++
				}
			}
			if len(o.Label.Tags) > 0 {
				tagN++
				if o.TagsOK {
					tagOK++
				}
			}
			if o.Label.Identity != "" {
				idN++
				if o.IDOK {
					idOK++
				}
			}
		}
		fmt.Printf("\n== %s: role %d/%d, tags %d/%d, identity %d/%d, errors %d\n", t, roleOK, roleN, tagOK, tagN, idOK, idN, errs)
		keys := make([]string, 0, len(confusion))
		for k := range confusion {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("   %-28s %d\n", k, confusion[k])
		}
		for _, o := range items {
			switch {
			case o.Err != nil:
				fmt.Printf("   ERR  %s: %v\n", o.Label.Path, o.Err)
			case len(o.Problems) > 0:
				fmt.Printf("   MISS %s: %s | %q tags=%v\n", o.Label.Path, strings.Join(o.Problems, "; "), o.Result.Description, tagKeys(o.Result))
			}
		}
	}
}

func tagKeys(r *analysis.Result) []string {
	var ks []string
	for _, t := range r.Tags {
		ks = append(ks, fmt.Sprintf("%s:%.2f", t.Key, t.Confidence))
	}
	return ks
}

func readLabels(path string) ([]label, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var labels []label
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		for len(cols) < 5 {
			cols = append(cols, "")
		}
		if cols[0] == "" || cols[1] == "" || cols[2] == "" {
			return nil, fmt.Errorf("%s:%d: need path, template and roles", path, n)
		}
		l := label{Path: cols[0], Template: cols[1], Roles: strings.Split(cols[2], "|"), Identity: strings.TrimSpace(cols[4])}
		if tags := strings.TrimSpace(cols[3]); tags != "" {
			l.Tags = strings.Split(tags, ",")
		}
		labels = append(labels, l)
	}
	return labels, sc.Err()
}

func writeAnswers(path string, results []outcome) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, o := range results {
		rec := map[string]any{"path": o.Label.Path, "template": o.Label.Template, "result": o.Result,
			"cost_usd": o.Usage.CostUSD, "latency_ms": o.Usage.Latency.Milliseconds(), "problems": o.Problems}
		if o.Err != nil {
			rec["error"] = o.Err.Error()
		}
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}
