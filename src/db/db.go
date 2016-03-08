//
// Copyright 2015 IBM Corp. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"cfg"
)

type JobStatus int

var menagerieDb *sql.DB

const (
	JobReceived JobStatus = 0
	JobRunning  JobStatus = 1
	JobFail     JobStatus = 2
	JobSuccess  JobStatus = 3
)

func getDb() (db *sql.DB, err error) {
	if menagerieDb == nil {
		db, err = cfg.NewMysqlDb()
		if err != nil {
			return nil, err
		} else {
			menagerieDb = db
		}
	}
	return menagerieDb, nil
}

func dbExec(q string, args ...interface{}) (sql.Result, error) {
	db, err := getDb()
	if err != nil {
		return nil, err
	}
	res, err := db.Exec(q, args...)
	if err != nil {
		return nil, fmt.Errorf("Error executing query (%q %q): %s", q, args, err)
	} else if nrows, err := res.RowsAffected(); err != nil {
		return nil, fmt.Errorf("Error getting number of rows: %s", err)
	} else if nrows != 1 {
		return nil, fmt.Errorf("Unexpected number of lines updated (1 != %d)", nrows)
	}
	return res, err
}

func JobCreate(engine string,filename string) (jid int64, err error) {
	res, err := dbExec("INSERT INTO jobs SET status='RECEIVED', engine=?, created=NOW(),filename=?", engine,filename)
	if err != nil {
		return -1, err
	}
	insertID, err := res.LastInsertId()
	return insertID, err
}

func JobSetStarted(jid int64) error {
	_, err := dbExec("UPDATE jobs SET status='RUNNING', started=NOW() WHERE id=?", jid)
	return err
}

func JobSetSuccess(jid int64) error {
	_, err := dbExec("UPDATE jobs SET status='SUCCESS', finished=NOW() WHERE id=?", jid)
	return err
}

func JobSetError(jid int64, errStr string) error {
	_, err := dbExec("UPDATE jobs SET status='FAIL', finished=NOW(), error=? WHERE id=?", errStr, jid)
	return err
}

func getJobStatuses(m map[string]JobStatus) []string {
	keys := make([]string, 0, len(m))
	for k, _ := range m {
		keys = append(keys, k)
	}
	return keys
}

var str2JobStatusMap = map[string]JobStatus{
	"RECEIVED": JobReceived,
	"RUNNING":  JobRunning,
	"FAIL":     JobFail,
	"SUCCESS":  JobSuccess,
}
var jobStatuses = getJobStatuses(str2JobStatusMap)

func str2JobStatus(s string) (JobStatus, error) {
	if ret, exists := str2JobStatusMap[s]; exists {
		return ret, nil
	}
	return JobReceived, fmt.Errorf("Unknown job status (%s)", s)
}

func JobGetStatus(jid int64) (st JobStatus, err error) {
	db, err := getDb()
	if err != nil {
		return -1, err
	}
	var statusStr string
	if err = db.QueryRow("SELECT status FROM jobs WHERE id=?", jid).Scan(&statusStr); err != nil {
		return -1, err
	}
	return str2JobStatus(statusStr)
}

func rows2MapsArray(rows *sql.Rows) (maps []map[string]interface{}, err error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	vals := make([]interface{}, len(cols)) // values
	ptrs := make([]interface{}, len(cols)) // Pointers to values
	for i := range vals {
		ptrs[i] = &vals[i] // Initialize pointer
	}

	maps = []map[string]interface{}{}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil { // Scan will fill in 'vals' using 'ptrs'
			return nil, err
		}
		job := map[string]interface{}{}
		for i, col := range cols {
			if b, isBytes := vals[i].([]byte); isBytes {
				job[col] = string(b) // Convert []byte to string. No one wants []byte values.
			} else {
				job[col] = vals[i]
			}
		}
		maps = append(maps, job)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return maps, nil
}

func queryRows2Maps(q string, args ...interface{}) (maps []map[string]interface{}, err error) {
	db, err := getDb()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows2MapsArray(rows)
}

func appendCond(cond, subCond string) string {
	if cond != "" {
		return cond + " AND " + subCond
	}
	return subCond
}

func genJobsFilterCond(engine string, statuses []string) (q string, args []interface{}) {
	var cond string
	if engine != "" {
		cond = "engine = ?"
		args = []interface{}{engine}
	}
	if len(statuses) > 0 {
		cond = appendCond(cond, "status in ("+strings.Repeat("?,", len(statuses)-1)+"?)")
		for _, s := range statuses {
			args = append(args, s)
		}
	}
	return cond, args
}

func genJobsQuery(maxIdx int64, limit int64, page int64, engine string, statuses []string) (q string, args []interface{}) {
	cond := "id <= ?"
	condArgs := make([]interface{}, 1, 10)
	condArgs[0] = maxIdx
	if fc, fcArgs := genJobsFilterCond(engine, statuses); fc != "" {
		cond += " AND " + fc
		condArgs = append(condArgs, fcArgs...)
	}
	q = fmt.Sprintf("SELECT * FROM jobs WHERE %s ORDER BY id DESC LIMIT ?,?", cond)
	args = append(condArgs, (page-1)*limit, limit)
	return q, args
}

func GetPagination(engine string, statuses []string, minID int64) (pagination []map[string]interface{}, err error) {
	cond, args := genJobsFilterCond(engine, statuses)
	if minID > 0 {
		cond = appendCond(cond, "id >= ?")
		args = append(args, minID)
	}
	q := "SELECT COUNT(*) AS count, MAX(id) AS max_index FROM jobs"
	if cond != "" {
		q += " WHERE " + cond
	}

	if pagination, err = queryRows2Maps(q, args...); err != nil {
		return nil, err
	}
	return pagination, nil
}

func GetJobs(maxIdx int64, limit int64, page int64, engine string, statuses []string) (jobs []map[string]interface{}, err error) {
	q, args := genJobsQuery(maxIdx, limit, page, engine, statuses)
	jobs, err = queryRows2Maps(q, args...)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func GetEngineStats(engines []string) (stats map[string]map[string]int64, err error) {
	db, err := getDb()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query("SELECT engine, status, count(*) FROM jobs GROUP BY engine, status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stats = map[string]map[string]int64{}
	for _, e := range engines {
		m := map[string]int64{}
		for _, s := range jobStatuses {
			m[s] = 0
		}
		stats[e] = m
	}
	var engine, status string
	var count int64
	for rows.Next() {
		if err = rows.Scan(&engine, &status, &count); err != nil {
			return nil, err
		}
		if s, exists := stats[engine]; exists {
			if _, exists := s[status]; exists {
				s[status] = count
			}
		}
	}
	return stats, nil
}
