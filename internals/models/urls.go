package models

import (
	"database/sql"
	"errors"
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

func (m *ShortenerDataModel) Insert(original string, shortened string, clicks int) (int, error) {
	statement := `INSERT INTO urls (original_url, shortened_url, clicks) VALUES (?, ?, ?)`
	result, err := m.DB.Exec(statement, original, shortened, clicks)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return int(rowsAffected), nil
}

func (m *ShortenerDataModel) Get(shortened string) (string, error) {
	statement := `SELECT original_url FROM urls WHERE shortened_url = ?`
	var original string
	row := m.DB.QueryRow(statement, shortened)
	err := row.Scan(&original)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var ErrNoRecord = errors.New("models: no matching record found")
			return "", ErrNoRecord
		} else {
			return "", err
		}
	}

	return original, nil
}

func (m *ShortenerDataModel) UpdateClicks(shortened string) error {
	statement := `UPDATE urls SET clicks = clicks + 1 WHERE shortened_url = ?`
	_, err := m.DB.Exec(statement, shortened)
	if err != nil {
		return err
	}

	return nil
}
