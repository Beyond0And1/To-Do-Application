package main

import (
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/*.gohtml static/*
var embeddedFS embed.FS

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Notes     string    `json:"notes"`
	Priority  Priority  `json:"priority"`
	Due       string    `json:"due"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	mu     sync.Mutex
	file   string
	nextID int
	items  []Todo
}

func NewStore(path string) *Store { return &Store{file: path} }

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.file)
	if errors.Is(err, os.ErrNotExist) {
		s.items = nil
		s.nextID = 0
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if err := dec.Decode(&s.items); err != nil {
		return err
	}
	max := 0
	for _, t := range s.items {
		if t.ID > max {
			max = t.ID
		}
	}
	s.nextID = max
	return nil
}

func (s *Store) saveLocked() error {
	f, err := os.Create(s.file)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(s.items)
}

func (s *Store) All() []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Todo, len(s.items))
	copy(cp, s.items)
	return cp
}

func (s *Store) Add(title, notes string, p Priority, due string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("title required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	now := time.Now()
	s.items = append(s.items, Todo{
		ID:        s.nextID,
		Title:     strings.TrimSpace(title),
		Notes:     strings.TrimSpace(notes),
		Priority:  p,
		Due:       strings.TrimSpace(due),
		Done:      false,
		CreatedAt: now,
		UpdatedAt: now,
	})
	return s.saveLocked()
}

func (s *Store) Toggle(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].Done = !s.items[i].Done
			s.items[i].UpdatedAt = time.Now()
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) Update(id int, title, notes string, p Priority, due string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			if strings.TrimSpace(title) != "" {
				s.items[i].Title = strings.TrimSpace(title)
			}
			s.items[i].Notes = strings.TrimSpace(notes)
			s.items[i].Priority = p
			s.items[i].Due = strings.TrimSpace(due)
			s.items[i].UpdatedAt = time.Now()
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Store) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dst := s.items[:0]
	for _, t := range s.items {
		if t.ID != id {
			dst = append(dst, t)
		}
	}
	s.items = dst
	return s.saveLocked()
}

func (s *Store) ClearCompleted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dst := s.items[:0]
	for _, t := range s.items {
		if !t.Done {
			dst = append(dst, t)
		}
	}
	s.items = dst
	return s.saveLocked()
}

var tpl *template.Template

func mustPrepareTemplates() {
	tpl = template.Must(template.ParseFS(embeddedFS, "templates/*.gohtml"))
}

func main() {
	dataFile := filepath.Join(".", "todos.json")
	store := NewStore(dataFile)
	if err := store.Load(); err != nil {
		log.Printf("load store: %v", err)
	}

	mustPrepareTemplates()

	staticFS, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		log.Fatalf("static fs sub: %v", err)
	}
	staticHandler := http.FileServer(http.FS(staticFS))

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", staticHandler))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		items := store.All()
		if err := tpl.ExecuteTemplate(w, "index.gohtml", items); err != nil {
			log.Println("template exec:", err)
		}
	})

	mux.HandleFunc("/add", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		_ = r.ParseForm()
		title := r.Form.Get("title")
		notes := r.Form.Get("notes")
		prio := Priority(r.Form.Get("priority"))
		due := r.Form.Get("due")
		if prio != PriorityLow && prio != PriorityMedium && prio != PriorityHigh {
			prio = PriorityLow
		}
		if err := store.Add(title, notes, prio, due); err != nil {
			log.Println("add:", err)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	mux.HandleFunc("/toggle", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		_ = r.ParseForm()
		id, _ := strconv.Atoi(r.Form.Get("id"))
		if err := store.Toggle(id); err != nil {
			log.Println("toggle:", err)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	mux.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		_ = r.ParseForm()
		id, _ := strconv.Atoi(r.Form.Get("id"))
		title := r.Form.Get("title")
		notes := r.Form.Get("notes")
		prio := Priority(r.Form.Get("priority"))
		due := r.Form.Get("due")
		if prio != PriorityLow && prio != PriorityMedium && prio != PriorityHigh {
			prio = PriorityLow
		}
		if err := store.Update(id, title, notes, prio, due); err != nil {
			log.Println("update:", err)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	mux.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		_ = r.ParseForm()
		id, _ := strconv.Atoi(r.Form.Get("id"))
		if err := store.Delete(id); err != nil {
			log.Println("delete:", err)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	mux.HandleFunc("/clear-completed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if err := store.ClearCompleted(); err != nil {
			log.Println("clear:", err)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
