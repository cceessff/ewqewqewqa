package pkg

import (
	"database/sql"
	"fmt"
)

type Record struct {
	Id          int    `json:"id"`
	Domain      string `json:"domain"`
	Path        string `json:"path"`
	UserAgent   string `json:"user_agent"`
	Spider      string `json:"spider"`
	CreatedTime int64  `json:"created_time"`
}

func (dao *Dao) AddRecord(record *Record) error {
	insertSql := `insert  into record(domain,path,user_agent,spider,created_time)values (?,?,?,?,?)`
	_, err := dao.Exec(insertSql, record.Domain, record.Path, record.UserAgent, record.Spider, record.CreatedTime)
	if err != nil {
		return err
	}
	return nil
}

func (dao *Dao) DelRecord(startTime, endTime int64) error {
	deleteSql := `delete from record where created_time>? and created_time<?`
	_, err := dao.Exec(deleteSql, startTime, endTime)
	if err != nil {
		return err
	}
	return nil
}

func (dao *Dao) recordList(domain string, startTime int64, endTime int64, page int, limit int) ([]Record, error) {
	start := (page - 1) * limit
	var rows *sql.Rows
	var err error
	var conditions []string
	if domain != "" {
		conditions = append(conditions, fmt.Sprintf("domain='%s'", domain))
	}
	if startTime > 0 {
		conditions = append(conditions, fmt.Sprintf("created_time>=%d", startTime))
	}
	if endTime > 0 {
		conditions = append(conditions, fmt.Sprintf("created_time<%d", endTime))
	}
	where := ""
	if len(conditions) > 0 {
		for i, condition := range conditions {
			if i == 0 {
				where += "where " + condition
			} else {
				where += " and " + condition
			}
		}

	}
	querySql := fmt.Sprintf("select * from record order by id desc limit %d,%d ", start, limit)
	if where != "" {
		querySql = fmt.Sprintf("select * from record %s order by id desc limit %d,%d ", where, start, limit)
	}
	rows, err = dao.Query(querySql)

	if err != nil {
		return nil, err
	}
	var results = make([]Record, 0)
	for rows.Next() {
		var record Record
		err := rows.Scan(
			&record.Id, &record.Domain, &record.Path,
			&record.UserAgent, &record.Spider, &record.CreatedTime)
		if err != nil {
			return nil, err
		}
		record.Path = Escape(record.Path)
		record.UserAgent = Escape(record.UserAgent)
		results = append(results, record)
	}
	_ = rows.Close()
	return results, nil
}

func (dao *Dao) recordCount(domain string, startTime int64, endTime int64) (int, error) {
	var conditions []string
	if domain != "" {
		conditions = append(conditions, fmt.Sprintf("domain='%s'", domain))
	}
	if startTime > 0 {
		conditions = append(conditions, fmt.Sprintf("created_time>=%d", startTime))
	}
	if endTime > 0 {
		conditions = append(conditions, fmt.Sprintf("created_time<%d", endTime))
	}
	where := ""
	if len(conditions) > 0 {
		for i, condition := range conditions {
			if i == 0 {
				where += "where " + condition
			} else {
				where += " and " + condition
			}
		}

	}
	querySql := "select count(*) as count from record"
	if where != "" {
		querySql = fmt.Sprintf("select count(*) as count from record %s", where)
	}
	count := 0
	row := dao.QueryRow(querySql)
	err := row.Scan(&count)

	if err == nil || err == sql.ErrNoRows {
		return count, nil
	}
	return 0, err
}

func createRecordTable(db *sql.DB) error {
	rs, err := db.Query(`SELECT count(*) as count FROM sqlite_master WHERE type='table' AND name = 'record'`)
	if err != nil {
		return err
	}
	var count int
	rs.Next()
	rs.Scan(&count)
	rs.Close()
	if count == 0 {
		_, err = db.Exec(`create table if not exists record  (
			id integer primary key AUTOINCREMENT,
			domain varchar(30) not null,
			path varchar(255),
			user_agent  varchar(255),
			spider varchar(30),
			created_time integer(11) default 0
			)`)

		if err != nil {
			return err
		}
		_, err = db.Exec("create index IF NOT EXISTS domain on record(domain)")
		if err != nil {
			return err
		}
		_, err = db.Exec("create index IF NOT EXISTS created_time on record(created_time)")
	}

	return err
}
