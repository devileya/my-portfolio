// Package seed loads the initial portfolio content from seed/data.json and
// uploads the referenced images to Garage. It runs once, on an empty
// database; after that the dashboard is the source of truth.
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/devileya/my-portfolio/internal/content"
	"github.com/devileya/my-portfolio/internal/storage"
)

type file struct {
	Profile struct {
		content.Profile
		Avatar string `json:"avatar"`
	} `json:"profile"`
	Projects []struct {
		content.Project
		Image string `json:"image"`
	} `json:"projects"`
	Experiences []content.Experience `json:"experiences"`
	Skills      []content.Skill      `json:"skills"`
}

type Options struct {
	Dir    string // directory holding data.json and images/
	CVPath string // optional PDF to upload as the CV
	Force  bool   // wipe existing content first
}

func Run(ctx context.Context, db *pgxpool.Pool, repo *content.Repo, store *storage.Store, opt Options) error {
	if store == nil {
		slog.Warn("object storage not configured: seeding text only; re-run with -force once S3_* is set to attach images and the CV")
	}
	var n int
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		return err
	}
	if n > 0 && !opt.Force {
		return fmt.Errorf("database already has %d projects; pass -force to replace all content", n)
	}
	if opt.Force {
		if _, err := db.Exec(ctx, `TRUNCATE projects, experiences, skills RESTART IDENTITY`); err != nil {
			return err
		}
	}

	raw, err := os.ReadFile(filepath.Join(opt.Dir, "data.json"))
	if err != nil {
		return err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("data.json: %w", err)
	}

	uploaded := map[string]string{}
	upload := func(name string) (string, error) {
		if name == "" || store == nil {
			return "", nil
		}
		if key, ok := uploaded[name]; ok {
			return key, nil
		}
		data, err := os.ReadFile(filepath.Join(opt.Dir, "images", name))
		if err != nil {
			return "", err
		}
		ct := http.DetectContentType(data)
		obj, err := store.Put(ctx, "images", filepath.Ext(name), ct, data)
		if err != nil {
			return "", err
		}
		slog.Info("uploaded", "file", name, "key", obj.Key)
		uploaded[name] = obj.Key
		return obj.Key, nil
	}

	p := f.Profile.Profile
	if p.AvatarKey, err = upload(f.Profile.Avatar); err != nil {
		return err
	}
	if err := repo.UpdateProfile(ctx, p); err != nil {
		return err
	}

	for _, sp := range f.Projects {
		pr := sp.Project
		pr.Published = true
		if pr.ImageKey, err = upload(sp.Image); err != nil {
			return err
		}
		if _, err := repo.SaveProject(ctx, pr); err != nil {
			return fmt.Errorf("project %q: %w", pr.Title, err)
		}
	}
	for _, e := range f.Experiences {
		e.Published = true
		if _, err := repo.SaveExperience(ctx, e); err != nil {
			return fmt.Errorf("experience %q: %w", e.Title, err)
		}
	}
	for _, s := range f.Skills {
		if _, err := repo.SaveSkill(ctx, s); err != nil {
			return fmt.Errorf("skill %q: %w", s.Name, err)
		}
	}

	if opt.CVPath != "" && store != nil {
		data, err := os.ReadFile(opt.CVPath)
		if err != nil {
			return err
		}
		obj, err := store.Put(ctx, "cv", ".pdf", "application/pdf", data)
		if err != nil {
			return err
		}
		if err := repo.SetCV(ctx, obj.Key, "Arif-Fadly-Siregar-CV.pdf", obj.Size, obj.SHA256); err != nil {
			return err
		}
		slog.Info("uploaded cv", "key", obj.Key)
	}

	slog.Info("seed complete", "projects", len(f.Projects), "experiences", len(f.Experiences), "skills", len(f.Skills))
	return nil
}
