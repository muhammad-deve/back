package migrations

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_4044198014")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"indexes": [
				"CREATE UNIQUE INDEX ` + "`" + `idx_HyuDyZUmYy` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `imdb_id` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_CZhsj9e227` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `type` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_t695AScJAz` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `genre_id` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_i5aSvrVrWA` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `released_year` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_NjG8qv1WxY` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `viewed` + "`" + `)"
			]
		}`), &collection); err != nil {
			return err
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("pbc_4044198014")
		if err != nil {
			return err
		}

		// update collection data
		if err := json.Unmarshal([]byte(`{
			"indexes": [
				"CREATE UNIQUE INDEX ` + "`" + `idx_HyuDyZUmYy` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `imdb_id` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_CZhsj9e227` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `type` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_t695AScJAz` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `genre_id` + "`" + `)",
				"CREATE INDEX ` + "`" + `idx_i5aSvrVrWA` + "`" + ` ON ` + "`" + `movies` + "`" + ` (` + "`" + `released_year` + "`" + `)"
			]
		}`), &collection); err != nil {
			return err
		}

		return app.Save(collection)
	})
}
