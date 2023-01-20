package pkg

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type SiteConfig struct {
	Id                  int      `json:"id"`
	Domain              string   `json:"domain"`
	Url                 string   `json:"url"`
	IndexTitle          string   `json:"index_title"`
	IndexKeywords       string   `json:"index_keywords"`
	IndexDescription    string   `json:"index_description"`
	Finds               []string `json:"finds"`
	Replaces            []string `json:"replaces"`
	TraditionalReplaces []string `json:"traditional_replaces"`
	NeedJs              bool     `json:"need_js"`
	S2t                 bool     `json:"s2t"`
	TitleReplace        bool     `json:"title_replace"`
	H1Replace           string   `json:"h1replace"`
	CacheTime           int64    `json:"cache_time"`
	CacheEnable         bool     `json:"cache_enable"`
	BaiduPushKey        string   `json:"baidu_push_key"`
	SmPushKey           string   `json:"sm_push_key"`
}

type Dao struct {
	db *sql.DB
}

func NewDao() (*Dao, error) {
	dao := &Dao{}
	db, err := sql.Open("sqlite3", "config/data.db")
	if err != nil {
		return nil, err
	}
	dao.db = db
	return dao, nil
}
func (dao *Dao) GetOne(domain string) (SiteConfig, error) {
	domain = strings.TrimSpace(domain)
	var siteConfig SiteConfig
	rs, err := dao.db.Query("select id,domain,url,index_title,index_keywords,index_description,finds,replaces,need_js,s2t,cache_enable,title_replace,h1replace,cache_time,baidu_push_key,sm_push_key from website_config where domain=?", domain)
	if err != nil {
		return siteConfig, err
	}

	if rs.Next() {
		var findsStr, replStr string
		err = rs.Scan(
			&siteConfig.Id,
			&siteConfig.Domain,
			&siteConfig.Url,
			&siteConfig.IndexTitle,
			&siteConfig.IndexKeywords,
			&siteConfig.IndexDescription,
			&findsStr, &replStr, &siteConfig.NeedJs, &siteConfig.S2t,
			&siteConfig.CacheEnable, &siteConfig.TitleReplace, &siteConfig.H1Replace,
			&siteConfig.CacheTime, &siteConfig.BaiduPushKey, &siteConfig.SmPushKey)
		if err != nil {
			return siteConfig, err
		}
		siteConfig.Finds = strings.Split(findsStr, ";")
		siteConfig.Replaces = strings.Split(replStr, ";")

	}
	err = rs.Close()
	if err != nil {
		return siteConfig, err
	}
	if siteConfig.Id == 0 {
		return siteConfig, errors.New("无搜索结果")
	}
	return siteConfig, nil

}
func (dao *Dao) DeleteOne(id int) error {
	_, err := dao.db.Exec("delete from website_config where id=?", id)
	if err != nil {
		return err
	}
	return nil
}
func (dao *Dao) GetAll() ([]*SiteConfig, error) {
	rs, err := dao.db.Query("select id, domain,url,index_title,index_keywords,index_description,finds,replaces,need_js,s2t,cache_enable,title_replace,h1replace,cache_time,baidu_push_key,sm_push_key from website_config")
	if err != nil {
		return nil, err
	}
	var results = make([]*SiteConfig, 0)
	for rs.Next() {
		var siteConfig SiteConfig
		var findsStr, replStr string
		err := rs.Scan(
			&siteConfig.Id, &siteConfig.Domain, &siteConfig.Url,
			&siteConfig.IndexTitle, &siteConfig.IndexKeywords, &siteConfig.IndexDescription,
			&findsStr, &replStr, &siteConfig.NeedJs, &siteConfig.S2t, &siteConfig.CacheEnable,
			&siteConfig.TitleReplace, &siteConfig.H1Replace, &siteConfig.CacheTime,
			&siteConfig.BaiduPushKey, &siteConfig.SmPushKey)
		if err != nil {
			return nil, err
		}
		siteConfig.Finds = strings.Split(findsStr, ";")
		siteConfig.Replaces = strings.Split(replStr, ";")
		results = append(results, &siteConfig)
	}
	_ = rs.Close()
	return results, nil

}
func (dao *Dao) addOne(data SiteConfig) error {
	insertSql := `insert  into website_config(domain,url,index_title,index_keywords,index_description,finds,replaces,need_js,s2t,cache_enable,title_replace,h1replace,cache_time,baidu_push_key,sm_push_key)values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	_, err := dao.db.Exec(insertSql, data.Domain, data.Url, data.IndexTitle, data.IndexKeywords, data.IndexDescription, strings.Join(data.Finds, ";"), strings.Join(data.Replaces, ";"), data.NeedJs, data.S2t, data.CacheEnable, data.TitleReplace, data.H1Replace, data.CacheTime, data.BaiduPushKey, data.SmPushKey)
	if err != nil {
		return err
	}
	return nil
}
func (dao *Dao) UpdateById(data SiteConfig) error {
	updateSql := "update website_config set url=?,domain=?,index_title=?,index_keywords=?,index_description=?,finds=?,replaces=?,need_js=?,s2t=?,cache_enable=?,title_replace=?,h1replace=?,cache_time=?,baidu_push_key=?,sm_push_key=? where id=?"
	_, err := dao.db.Exec(updateSql, data.Url, data.Domain, data.IndexTitle, data.IndexKeywords, data.IndexDescription, strings.Join(data.Finds, ";"), strings.Join(data.Replaces, ";"), data.NeedJs, data.S2t, data.CacheEnable, data.TitleReplace, data.H1Replace, data.CacheTime, data.BaiduPushKey, data.SmPushKey, data.Id)
	if err != nil {
		return err
	}
	return nil

}
func (dao *Dao) GetByPage(page, limit int) ([]SiteConfig, error) {
	start := (page - 1) * limit
	querySql := fmt.Sprintf("select * from website_config limit %d,%d", start, limit)
	rs, err := dao.db.Query(querySql)
	if err != nil {
		return nil, err
	}
	var results = make([]SiteConfig, 0)
	for rs.Next() {
		var siteConfig SiteConfig
		var findsStr, replStr string
		err := rs.Scan(
			&siteConfig.Id, &siteConfig.Domain, &siteConfig.Url,
			&siteConfig.IndexTitle, &siteConfig.IndexKeywords, &siteConfig.IndexDescription,
			&findsStr, &replStr, &siteConfig.NeedJs, &siteConfig.S2t, &siteConfig.CacheEnable,
			&siteConfig.TitleReplace, &siteConfig.H1Replace, &siteConfig.CacheTime,
			&siteConfig.BaiduPushKey, &siteConfig.SmPushKey)
		if err != nil {
			return nil, err
		}
		siteConfig.Finds = strings.Split(findsStr, ";")
		siteConfig.Replaces = strings.Split(replStr, ";")

		results = append(results, siteConfig)
	}
	_ = rs.Close()
	return results, nil
}
func (dao *Dao) AddMulti(configs []*SiteConfig) error {
	tx, err := dao.db.Begin()
	if err != nil {
		return err
	}
	insetSql := `insert into website_config(domain,url,index_title,index_keywords,index_description,finds,replaces,need_js,s2t,cache_enable,title_replace,h1replace,cache_time,baidu_push_key,sm_push_key)values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	for _, data := range configs {
		_, err := tx.Exec(insetSql, data.Domain, data.Url, data.IndexTitle, data.IndexKeywords, data.IndexDescription, strings.Join(data.Finds, ";"), strings.Join(data.Replaces, ";"), data.NeedJs, data.S2t, data.CacheEnable, data.TitleReplace, data.H1Replace, data.CacheTime, data.BaiduPushKey, data.SmPushKey)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil

}
func (dao *Dao) MultiDel(domains []string) error {
	args := make([]interface{}, len(domains))
	for i, id := range domains {
		args[i] = id
	}
	delSql := `delete from website_config where domain in (?` + strings.Repeat(",?", len(args)-1) + `)`
	_, err := dao.db.Exec(delSql, args...)
	if err != nil {
		return err
	}
	return nil

}

func (dao *Dao) Count() (int, error) {
	countSql := `select count(*) as count from website_config`
	rs, err := dao.db.Query(countSql)
	if err != nil {
		return 0, err
	}
	var count int
	rs.Next()
	err = rs.Scan(&count)
	if err != nil {
		return 0, err
	}
	err = rs.Close()
	if err != nil {
		return 0, err
	}
	return count, nil

}
func (dao *Dao) ForbiddenWordReplace(forbiddenWord, replaceWord, splitWord string) ([]string, error) {
	forbiddenSql := "select domain,index_title from website_config where index_title like ?"
	rs, err := dao.db.Query(forbiddenSql, "%"+forbiddenWord+"%")
	if err != nil {
		return nil, err
	}
	var indexTitleArr = make(map[string]string)
	var temp string
	var tempDomain string
	for rs.Next() {
		err = rs.Scan(&tempDomain)
		if err != nil {
			return nil, err
		}
		err = rs.Scan(&temp)
		if err != nil {
			return nil, err
		}
		indexTitleArr[tempDomain] = temp
	}
	_ = rs.Close()
	if len(indexTitleArr) == 0 {
		return nil, errors.New("没有找到要替换的禁词")
	}
	var domainArr = make([]string, 0)
	updateSql := `update website_config set index_title=? where index_title=?`
	for domain, title := range indexTitleArr {
		if strings.Contains(title, forbiddenWord+splitWord) || strings.Contains(title, splitWord+forbiddenWord) {
			words := strings.Split(title, splitWord)
			for i, word := range words {
				if word == forbiddenWord {
					words[i] = replaceWord
				}
			}
			newTitle := strings.Join(words, splitWord)
			_, err := dao.db.Exec(updateSql, newTitle, title)
			if err != nil {
				return nil, err
			}
			dn := domain + "##" + newTitle
			domainArr = append(domainArr, dn)
		}
	}
	return domainArr, err
}

func (dao *Dao) AddRecord(record *Record) error {
	insertSql := `insert  into record(domain,path,user_agent,spider,created_time)values (?,?,?,?,?)`
	_, err := dao.db.Exec(insertSql, record.Domain, record.Path, record.UserAgent, record.Spider, record.CreatedTime)
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
	querySql := fmt.Sprintf("select * from record limit %d,%d", start, limit)
	if where != "" {
		querySql = fmt.Sprintf("select * from record %s limit %d,%d", where, start, limit)
	}
	rows, err = dao.db.Query(querySql)

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
		results = append(results, record)
	}
	_ = rows.Close()
	return results, nil
}

func (dao *Dao) recordCount(domain string, startTime int64, endTime int64, page int, limit int) (int, error) {
	start := (page - 1) * limit
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
	querySql := fmt.Sprintf("select count(*) as count from record limit %d,%d", start, limit)
	if where != "" {
		querySql = fmt.Sprintf("select count(*) as count from record %s limit %d,%d", where, start, limit)
	}
	count := 0
	row := dao.db.QueryRow(querySql)
	err := row.Scan(&count)
	if err != nil {
		return count, err
	}
	return count, nil
}

func InitTable() error {
	db, err := sql.Open("sqlite3", "config/data.db")
	if err != nil {
		return err
	}
	err = createSiteTable(db)
	if err != nil {
		return err
	}
	return createRecordTable(db)
}

func createSiteTable(db *sql.DB) error {
	rs, err := db.Query(`SELECT count(*) as count FROM sqlite_master WHERE type='table' AND name = 'website_config'`)
	if err != nil {
		return err
	}
	var count int
	rs.Next()
	rs.Scan(&count)
	rs.Close()
	if count == 0 {
		_, err = db.Exec(`create table if not exists website_config  (
		id integer primary key AUTOINCREMENT,
		domain varchar(30) not null unique ,
		url varchar(50),
		index_title varchar(50),
		index_keywords varchar(100),
		index_description varchar(255),
		finds varchar(100),
		replaces varchar(100),
		need_js boolean default false ,
		s2t boolean default false ,
		cache_enable boolean default true,
		title_replace boolean default false ,
		h1replace varchar(20),
		cache_time integer,
		baidu_push_key varchar(255),
		sm_push_key varchar(255)	
)`)

	}
	return err
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
