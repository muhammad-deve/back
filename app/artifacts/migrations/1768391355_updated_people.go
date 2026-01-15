package migrations

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_520427368")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"indexes": [
				"CREATE INDEX ` + "`" + `idx_vVxV7E3Q5E` + "`" + ` ON ` + "`" + `people` + "`" + ` (` + "`" + `movie_id` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_Z3zf821Y64` + "`" + ` ON ` + "`" + `people` + "`" + ` (` + "`" + `imdb_id` + "`" + `)"
			]
		}`), &collection); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_520427368")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"indexes": []
		}`), &collection); err != nil {
			return err
		}

		return app.Save(collection)
	})
}
