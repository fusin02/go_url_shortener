package models

import (
	"database/sql"
)

type Url struct {
	OriginalUrl, ShortenedUrl string
	Clicks                    int
}

type ShortenerDataModel struct {
	DB *sql.DB
}

// Retrieves the latest urls from the database
func (m *ShortenerDataModel) GetLatest() ([]*Url, error) {
	statement := `SELECT original_url, shortened_url, clicks FROM urls`
	rows, err := m.DB.Query(statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	urls := []*Url{}
	for rows.Next() {
		url := &Url{}
		err := rows.Scan(&url.OriginalUrl, &url.ShortenedUrl, &url.Clicks)
		if err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}
