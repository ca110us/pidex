// Command evalappearance measures filing by appearance: photos of known
// individuals (pets nobody labels) are shot one after another into an empty
// collection, and each is filed the way the pipeline files it: the vision
// model describes it, the closest-looking subjects are found by that
// description, and the match model compares photos of them.
//
// Labels are TSV, one photo per line: path, collection, individual. Each
// collection starts empty. A suggestion is confirmed or corrected the way a
// person would, with one tap; automatic filings are left as they are, wrong
// or not, as nobody reviews them.
//
//	OPENROUTER_API_KEY=... evalappearance -labels labels.tsv -images DIR \
//	    -template household_pets [-match MODEL] [-seed N] [-analysis cache.jsonl]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"pidex/internal/ai"
	"pidex/internal/analysis"
	"pidex/internal/imaging"
	"pidex/internal/looks"
	"pidex/internal/pipeline"
	"pidex/internal/search"
	"pidex/internal/teamconfig"
)

type photo struct {
	Path, Collection, Truth string
}

// subject is one filed in the simulated collection.
type subject struct {
	ID       uuid.UUID
	Name     string
	Features string
	Vec      []float32
	Truth    string
	Intro    string
	Manual   []string
	Auto     []string
	// Looks are the look service's vectors of its filed photos.
	Looks [][]float32
}

type outcome string

const (
	notSeen       outcome = "not seen as a subject"
	autoRight     outcome = "filed right"
	autoWrong     outcome = "FILED WRONG"
	newRight      outcome = "new, right"
	newSplit      outcome = "NEW BUT KNOWN (split)"
	suggestRight  outcome = "suggested right (1 tap)"
	suggestWrong  outcome = "suggested wrong (fixed)"
	unassigned    outcome = "left for a person"
	compareFailed outcome = "compare failed"
)

func main() {
	labelsPath := flag.String("labels", "", "labels TSV: path, collection, individual")
	dir := flag.String("images", ".", "directory the label paths are relative to")
	template := flag.String("template", "household_pets", "team config template")
	configPath := flag.String("config", "", "team config YAML file, instead of a template")
	vision := flag.String("model", "google/gemini-3.1-flash-lite", "model that describes each photo")
	match := flag.String("match", "", "model that compares photos (default: -model)")
	embed := flag.String("embed", "voyageai/voyage-4", "text embedding model for finding look-alikes")
	seed := flag.Uint64("seed", 1, "order the photos are shot in")
	cachePath := flag.String("analysis", "", "JSON lines cache of descriptions, shared across runs")
	maxEdge := flag.Int("max-edge", 1280, "long edge sent to the model, as in the pipeline")
	verbose := flag.Bool("v", false, "print every photo")
	boxesPath := flag.String("boxes", "", "TSV of each photo's subject box (path, x, y, w, h), as the phone finds it; compared photos are cut to it")
	lookURL := flag.String("look", "", "look service URL (vision/): photos surely alike by look are filed without the model, and candidates come by look first")
	lookAuto := flag.Float64("look-auto", pipeline.DefaultLookAutoSimilarity, "similarity that files by look alone")
	lookMargin := flag.Float64("look-margin", pipeline.DefaultLookMargin, "how much closer than the next subject it must be")
	flag.Parse()
	var lookClient *looks.Client
	if *lookURL != "" {
		lookClient = looks.New(*lookURL)
	}

	key := os.Getenv("OPENROUTER_API_KEY")
	if *labelsPath == "" || key == "" {
		fmt.Fprintln(os.Stderr, "usage: OPENROUTER_API_KEY=... evalappearance -labels labels.tsv -images DIR")
		os.Exit(2)
	}
	if *match == "" {
		*match = *vision
	}
	var cfg *teamconfig.Config
	if *configPath != "" {
		data, err := os.ReadFile(*configPath)
		check(err)
		cfg, err = teamconfig.ParseYAML(data)
		check(err)
	} else {
		var err error
		cfg, err = teamconfig.LoadTemplate(*template)
		check(err)
	}
	var st *teamconfig.SubjectType
	for i := range cfg.SubjectTypes {
		if cfg.SubjectTypes[i].ByAppearance() {
			st = &cfg.SubjectTypes[i]
		}
	}
	if st == nil {
		check(fmt.Errorf("template %s has no subject type told apart by appearance", *template))
	}
	photos, err := readLabels(*labelsPath)
	check(err)
	if *boxesPath != "" {
		check(readBoxes(*boxesPath))
	}

	log := slog.New(slog.DiscardHandler)
	matchCost := &usageRecorder{}
	router := func(model string, rec ai.Recorder) *ai.Router {
		return ai.NewRouter(rec, log, &ai.OpenAICompatible{Name: "openrouter", BaseURL: ai.OpenRouterBaseURL, APIKey: key,
			ModelID: model, ReportCost: true, Headers: map[string]string{"X-Title": "Pidex eval"}})
	}
	visionR, matchR := router(*vision, nil), router(*match, matchCost)
	embedder := &ai.OpenAICompatible{Name: "openrouter", BaseURL: ai.OpenRouterBaseURL, APIKey: key, ModelID: *embed,
		EmbeddingDimensions: search.EmbeddingDimensions, Headers: map[string]string{"X-Title": "Pidex eval"}}

	seen, describeCost := describeAll(visionR, cfg, photos, *dir, *maxEdge, *cachePath, *vision)

	byColl := map[string][]photo{}
	var colls []string
	for _, p := range photos {
		if byColl[p.Collection] == nil {
			colls = append(colls, p.Collection)
		}
		byColl[p.Collection] = append(byColl[p.Collection], p)
	}
	th := cfg.Thresholds()
	total := map[outcome]int{}
	var compareTime time.Duration
	var compares, byLookFiled, byLookRight int
	for _, c := range colls {
		list := byColl[c]
		rng := rand.New(rand.NewPCG(*seed, 7))
		rng.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
		var subjects []*subject
		var checks []pipeline.LookCheck
		counts := map[outcome]int{}
		fmt.Printf("\n== %s: %d photos of %d individuals\n", c, len(list), individuals(list))
		for _, p := range list {
			a := seen[p.Path]
			if a == nil {
				counts[notSeen]++
				if *verbose {
					fmt.Printf("  %-10s %-26s %s\n", p.Truth, notSeen, p.Path)
				}
				continue
			}
			vec := embedOne(embedder, a.Features)
			cands := closest(subjects, vec)
			var look []float32
			var matches []pipeline.LookMatch
			if lookClient != nil {
				img, err := cutOut(*dir, p.Path, 1024)
				check(err)
				l, err := lookClient.Embed(context.Background(), img)
				check(err)
				look = l.Vector
				cands, matches = byLook(subjects, cands, look)
			}
			v := pipeline.AppearanceVerdict{Match: -1, Confidence: 0.99}
			if len(cands) > 0 && pipeline.SureByLook(matches, cfg, *lookAuto, *lookMargin) {
				// cands start with the closest by look.
				v = pipeline.AppearanceVerdict{Match: 0, Confidence: 0.98, ModelConfidence: 0.98, ByLook: true, Look: matches[0].Similarity}
				byLookFiled++
				if cands[0].Truth == p.Truth {
					byLookRight++
				}
			} else if len(cands) > 0 {
				img, err := cutOut(*dir, p.Path, 1024)
				check(err)
				var cs []pipeline.AppearanceCandidate
				var vecs [][]float32
				for _, s := range cands {
					vecs = append(vecs, s.Vec)
				}
				for i, s := range cands {
					cs = append(cs, pipeline.AppearanceCandidate{ID: s.ID, DisplayName: s.Name, Appearance: s.Features, Refs: refs(s, *dir, pipeline.RefCount(vecs, i)), Vec: s.Vec})
				}
				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				v, err = pipeline.CompareAppearance(ctx, matchR, ai.CallMeta{Purpose: "eval"}, st, img, a, cs)
				cancel()
				if err == nil {
					v = pipeline.AppearanceRules(v, cs, cfg, checks)
				}
				compareTime += time.Since(start)
				compares++
				if err != nil {
					counts[compareFailed]++
					fmt.Printf("  compare failed: %v\n", err)
					continue
				}
			}
			var o outcome
			var filed *subject
			known := len(subjects)
			truthHas := func() *subject {
				for _, s := range subjects {
					if s.Truth == p.Truth {
						return s
					}
				}
				return nil
			}
			switch {
			case v.Match >= 0 && v.Confidence >= th.Suggest:
				pick := cands[v.Match]
				if v.Confidence >= th.AutoAssign {
					filed = pick
					o = map[bool]outcome{true: autoRight, false: autoWrong}[pick.Truth == p.Truth]
					pick.Auto = append(pick.Auto, p.Path)
					break
				}
				// One tap: confirmed, or corrected to the right one.
				if pick.Truth == p.Truth {
					o, filed = suggestRight, pick
				} else {
					o, filed = suggestWrong, truthHas()
				}
			case v.Match < 0 && v.Confidence >= th.Suggest:
				known := truthHas()
				s := &subject{ID: uuid.New(), Name: a.Label, Features: a.Features, Vec: vec, Truth: p.Truth, Intro: p.Path}
				if v.Confidence >= th.AutoAssign {
					o = map[bool]outcome{true: newRight, false: newSplit}[known == nil]
					subjects = append(subjects, s)
					break
				}
				if known == nil {
					o = suggestRight
					subjects = append(subjects, s)
				} else {
					o, filed = suggestWrong, known
				}
			default:
				o, filed = unassigned, truthHas()
				if filed == nil {
					subjects = append(subjects, &subject{ID: uuid.New(), Name: a.Label, Features: a.Features, Vec: vec, Truth: p.Truth, Intro: p.Path})
				}
			}
			if v.Confidence == 0 {
				unclear[p.Path] = true
			}
			// A person filing a photo the model named someone for checks
			// that answer; one filed without asking is not checked.
			if v.Match >= 0 && o != autoRight && o != autoWrong {
				final := uuid.New()
				if filed != nil {
					final = filed.ID
				}
				checks = append([]pipeline.LookCheck{{Verdict: cands[v.Match].ID, Final: final, Confidence: v.ModelConfidence, Session: uuid.New()}}, checks...)
			}
			if filed != nil && o != autoRight && o != autoWrong {
				filed.Manual = append(filed.Manual, p.Path)
			}
			// Its look joins the subject it landed in.
			if look != nil {
				switch {
				case filed != nil:
					filed.Looks = append(filed.Looks, look)
				case len(subjects) > known:
					subjects[len(subjects)-1].Looks = append(subjects[len(subjects)-1].Looks, look)
				}
			}
			counts[o]++
			if *verbose {
				fmt.Printf("  %-10s %-26s %.2f %s | %s\n", p.Truth, o, v.Confidence, filepath.Base(p.Path), v.Reason)
			}
		}
		report(counts, len(list))
		fmt.Printf("  subjects: %d for %d individuals\n", len(subjects), individuals(list))
		for o, n := range counts {
			total[o] += n
		}
	}
	fmt.Printf("\n== all\n")
	report(total, len(photos))
	if lookClient != nil {
		fmt.Printf("  filed by look alone: %d, %d right\n", byLookFiled, byLookRight)
	}
	fmt.Printf("  describe (%s): $%.4f; compare (%s): %d calls, $%.4f, %.1fs each\n",
		*vision, describeCost, *match, compares, matchCost.usage.CostUSD, compareTime.Seconds()/math.Max(1, float64(compares)))
}

func report(counts map[outcome]int, n int) {
	order := []outcome{autoRight, newRight, suggestRight, suggestWrong, unassigned, autoWrong, newSplit, notSeen, compareFailed}
	for _, o := range order {
		if counts[o] > 0 {
			fmt.Printf("  %-28s %3d  %4.0f%%\n", o, counts[o], 100*float64(counts[o])/float64(n))
		}
	}
	filed := counts[autoRight] + counts[newRight] + counts[autoWrong] + counts[newSplit]
	if filed > 0 {
		fmt.Printf("  automatic and right: %.0f%% of %d filed without asking\n", 100*float64(counts[autoRight]+counts[newRight])/float64(filed), filed)
	}
}

type usageRecorder struct {
	mu    sync.Mutex
	usage ai.Usage
}

func (u *usageRecorder) Record(_ context.Context, c ai.CallRecord) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.usage.CostUSD += c.Usage.CostUSD
}

// byLook puts first the subjects whose photos look most like look (closest
// first, as the pipeline's lookMatches does), then the rest of cands.
func byLook(subjects, cands []*subject, look []float32) ([]*subject, []pipeline.LookMatch) {
	type scored struct {
		s   *subject
		sim float64
	}
	var all []scored
	for _, s := range subjects {
		best := -1.0
		for _, l := range s.Looks {
			best = max(best, cosine(look, l))
		}
		if best > -1 {
			all = append(all, scored{s, best})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].sim > all[j].sim })
	all = all[:min(len(all), 6)]
	var out []*subject
	var matches []pipeline.LookMatch
	in := map[*subject]bool{}
	for _, x := range all {
		out = append(out, x.s)
		matches = append(matches, pipeline.LookMatch{SubjectID: x.s.ID, Similarity: x.sim})
		in[x.s] = true
	}
	for _, s := range cands {
		if len(out) == 6 {
			break
		}
		if !in[s] {
			out = append(out, s)
		}
	}
	return out, matches
}

// closest are the subjects nearest in description, as the pipeline ranks
// them.
func closest(subjects []*subject, vec []float32) []*subject {
	out := append([]*subject(nil), subjects...)
	sort.SliceStable(out, func(i, j int) bool { return cosine(out[i].Vec, vec) > cosine(out[j].Vec, vec) })
	return out[:min(len(out), 6)]
}

// unclear are the photos whose subject could not be made out.
var unclear = map[string]bool{}

// refs are a subject's photos as the pipeline picks them: clear ones first;
// among them, confirmed by a person, then the one that introduced it, then
// the latest filed.
func refs(s *subject, dir string, n int) [][]byte {
	var paths []string
	for i := len(s.Manual) - 1; i >= 0; i-- {
		paths = append(paths, s.Manual[i])
	}
	paths = append(paths, s.Intro)
	for i := len(s.Auto) - 1; i >= 0; i-- {
		paths = append(paths, s.Auto[i])
	}
	sort.SliceStable(paths, func(i, j int) bool { return !unclear[paths[i]] && unclear[paths[j]] })
	var out [][]byte
	for _, p := range paths[:min(len(paths), n)] {
		if b, err := cutOut(dir, p, 512); err == nil {
			out = append(out, b)
		}
	}
	return out
}

// boxes are where each photo's subject is, by path.
var boxes = map[string]imaging.Box{}

func readBoxes(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		cols := strings.Split(line, "\t")
		if len(cols) != 5 {
			continue
		}
		var b imaging.Box
		if _, err := fmt.Sscan(cols[1], &b.X); err != nil {
			return err
		}
		fmt.Sscan(cols[2], &b.Y)
		fmt.Sscan(cols[3], &b.W)
		fmt.Sscan(cols[4], &b.H)
		boxes[cols[0]] = b
	}
	return nil
}

// cutOut loads a photo cut to its subject when its box is known, as the
// pipeline does with the box the phone found.
func cutOut(dir, path string, edge int) ([]byte, error) {
	data, err := load(filepath.Join(dir, path), edge*2)
	if err != nil {
		return nil, err
	}
	if b, ok := boxes[path]; ok {
		return imaging.Crop(data, b, 0.15, edge)
	}
	return imaging.Fit(data, edge)
}

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i] * b[i])
		na += float64(a[i] * a[i])
		nb += float64(b[i] * b[i])
	}
	return dot / (math.Sqrt(na)*math.Sqrt(nb) + 1e-9)
}

func embedOne(e ai.TextEmbedder, text string) []float32 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	v, _, err := e.EmbedTexts(ctx, []string{text})
	if err != nil {
		fmt.Fprintln(os.Stderr, "embedding failed:", err)
		return nil
	}
	return v[0]
}

// describeAll runs the pipeline's analysis on every photo, in parallel and
// cached: it does not depend on what was filed before.
func describeAll(r *ai.Router, cfg *teamconfig.Config, photos []photo, dir string, maxEdge int, cachePath, model string) (map[string]*analysis.Appearance, float64) {
	type entry struct {
		Model, Path string
		Appearance  *analysis.Appearance
	}
	out := map[string]*analysis.Appearance{}
	done := map[string]bool{}
	if cachePath != "" {
		if f, err := os.Open(cachePath); err == nil {
			sc := bufio.NewScanner(f)
			sc.Buffer(nil, 1<<20)
			for sc.Scan() {
				var e entry
				if json.Unmarshal(sc.Bytes(), &e) == nil && e.Model == model {
					out[e.Path], done[e.Path] = e.Appearance, true
				}
			}
			f.Close()
		}
	}
	var mu sync.Mutex
	var cost float64
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	var fresh []entry
	for _, p := range photos {
		if done[p.Path] {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			img, err := load(filepath.Join(dir, p.Path), maxEdge)
			check(err)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			raw, usage, err := r.Structured(ctx, ai.CallMeta{Purpose: "eval"}, ai.StructuredRequest{
				Instructions: analysis.Instructions(cfg), Prompt: analysis.Prompt,
				Images: []ai.Image{{Data: img, MIMEType: "image/jpeg"}}, Schema: analysis.Schema(cfg), MaxOutputTokens: 1024,
			})
			var a *analysis.Appearance
			if err == nil {
				var res *analysis.Result
				if res, err = analysis.Parse(raw, cfg); err == nil {
					a = res.Appearance
				}
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "describe failed:", p.Path, err)
				return
			}
			mu.Lock()
			out[p.Path] = a
			cost += usage.CostUSD
			fresh = append(fresh, entry{Model: model, Path: p.Path, Appearance: a})
			mu.Unlock()
		}()
	}
	wg.Wait()
	if cachePath != "" && len(fresh) > 0 {
		f, err := os.OpenFile(cachePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		check(err)
		for _, e := range fresh {
			b, _ := json.Marshal(e)
			f.Write(append(b, '\n'))
		}
		f.Close()
	}
	return out, cost
}

func load(path string, edge int) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return imaging.Fit(data, edge)
}

func individuals(list []photo) int {
	seen := map[string]bool{}
	for _, p := range list {
		seen[p.Truth] = true
	}
	return len(seen)
}

func readLabels(path string) ([]photo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []photo
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			return nil, fmt.Errorf("label line needs path, collection, individual: %q", line)
		}
		out = append(out, photo{Path: cols[0], Collection: cols[1], Truth: cols[2]})
	}
	return out, sc.Err()
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
