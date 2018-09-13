package database

import (
	db "morningo/connections/database/mysql"
	"errors"
	"strconv"
	"strings"
	"morningo/config"
	"morningo/module/logger"
)

type Where struct {
	operation string
	field     string
	qmark     string
}

type Join struct {
	table     string
	fieldA    string
	operation string
	fieldB    string
}

type RawUpdate struct {
	expression string
	args       []interface{}
}

type Sql struct {
	fields    []string
	table     string
	wheres    []Where
	leftjoins []Join
	args      []interface{}
	order     string
	offset    string
	limit     string
	whereRaw  string
	updateRaw []RawUpdate
	tx        *db.SqlTxStruct
	statement string
}

type H map[string]interface{}

func newSql() *Sql {
	return &Sql{
		fields:    make([]string, 0),
		table:     "",
		args:      make([]interface{}, 0),
		wheres:    make([]Where, 0),
		leftjoins: make([]Join, 0),
		updateRaw: make([]RawUpdate, 0),
		tx:        nil,
	}
}

// *******************************
// process method
// *******************************

func Table(table string) *Sql {
	sql := newSql()
	sql.table = table
	return sql
}

func SetTx(tx *db.SqlTxStruct) *Sql {
	sql := newSql()
	sql.tx = tx
	return sql
}

func (sql *Sql) Table(table string) *Sql {
	sql.table = table
	return sql
}

func (sql *Sql) Select(fields ...string) *Sql {
	sql.fields = fields
	return sql
}

func (sql *Sql) OrderBy(filed string, order string) *Sql {
	sql.order = filed + " " + order
	return sql
}

func (sql *Sql) Skip(offset int) *Sql {
	sql.offset = strconv.Itoa(offset)
	return sql
}

func (sql *Sql) Take(take int) *Sql {
	sql.limit = strconv.Itoa(take)
	return sql
}

func (sql *Sql) Where(field string, operation string, arg interface{}) *Sql {
	sql.wheres = append(sql.wheres, Where{
		field:     field,
		operation: operation,
		qmark:     "?",
	})
	sql.args = append(sql.args, arg)
	return sql
}

func (sql *Sql) WhereIn(field string, arg []interface{}) *Sql {
	qmark := "("
	for i := 0; i < len(arg); i++ {
		qmark += "?, "
	}
	qmark = qmark[:len(qmark)-2] + ")"

	sql.wheres = append(sql.wheres, Where{
		field:     field,
		operation: "in",
		qmark:     qmark,
	})
	sql.args = append(sql.args, arg...)
	return sql
}

func (sql *Sql) WhereNotIn(field string, arg []interface{}) *Sql {
	qmark := "("
	for i := 0; i < len(arg); i++ {
		qmark += "?, "
	}
	qmark = qmark[:len(qmark)-2] + ")"

	sql.wheres = append(sql.wheres, Where{
		field:     field,
		operation: "not in",
		qmark:     qmark,
	})
	sql.args = append(sql.args, arg...)
	return sql
}

func (sql *Sql) Find(arg interface{}) (map[string]interface{}, error) {
	return sql.Where("id", "=", arg).First()
}

func (sql *Sql) Count() (int64, error) {
	var (
		res map[string]interface{}
		err error
	)
	if res, err = sql.Select("count(*)").First(); err != nil {
		return 0, err
	}
	return res["count(*)"].(int64), nil
}

func (sql *Sql) WhereRaw(raw string) *Sql {
	sql.whereRaw = raw
	return sql
}

func (sql *Sql) UpdateRaw(raw string, args ...interface{}) *Sql {
	sql.updateRaw = append(sql.updateRaw, RawUpdate{
		expression: raw,
		args:       args,
	})
	return sql
}

func (sql *Sql) LeftJoin(table string, fieldA string, operation string, fieldB string) *Sql {
	sql.leftjoins = append(sql.leftjoins, Join{
		fieldA:    fieldA,
		fieldB:    fieldB,
		table:     table,
		operation: operation,
	})
	return sql
}

// *******************************
// terminal method
// -------------------------------
// sql args order:
// update ... => where ...
// *******************************

func (sql *Sql) First() (map[string]interface{}, error) {
	defer sql.log()

	sql.statement = "select " + sql.getFields() + " from " + sql.table + sql.getJoins() + sql.getWheres() +
		sql.getOrderBy() + sql.getLimit() + sql.getOffset()

	res, _ := db.Query(sql.statement, sql.args...)

	if len(res) < 1 {
		return nil, errors.New("out of index")
	}
	return res[0], nil
}

func (sql *Sql) All() ([]map[string]interface{}, error) {
	defer sql.log()

	sql.statement = "select " + sql.getFields() + " from " + sql.table + sql.getJoins() + sql.getWheres() +
		sql.getOrderBy() + sql.getLimit() + sql.getOffset()

	res, _ := db.Query(sql.statement, sql.args...)

	return res, nil
}

func (sql *Sql) Update(values H) (int64, error) {
	defer sql.log()

	sql.prepareUpdate(values)

	if sql.tx != nil {
		res, err := sql.tx.Exec(sql.statement, sql.args...)

		if err != nil {
			return 0, err
		}

		return res.LastInsertId()
	}

	res := db.Exec(sql.statement, sql.args...)

	if affectRow, _ := res.RowsAffected(); affectRow < 1 {
		return 0, errors.New("no affect row")
	}

	return res.LastInsertId()
}

func (sql *Sql) Exec() (int64, error) {
	defer sql.log()

	sql.prepareUpdate(H{})

	if sql.tx != nil {
		res, err := sql.tx.Exec(sql.statement, sql.args...)

		if err != nil {
			return 0, err
		}

		return res.LastInsertId()
	}

	res := db.Exec(sql.statement, sql.args...)

	if affectRow, _ := res.RowsAffected(); affectRow < 1 {
		return 0, errors.New("no affect row")
	}

	return res.LastInsertId()
}

func (sql *Sql) Insert(values H) (int64, error) {
	defer sql.log()

	sql.prepareInsert(values)

	if sql.tx != nil {
		res, err := sql.tx.Exec(sql.statement, sql.args...)

		if err != nil {
			return 0, errors.New("no affect row")
		}

		return res.LastInsertId()
	}

	res := db.Exec(sql.statement, sql.args...)

	if affectRow, _ := res.RowsAffected(); affectRow < 1 {
		return 0, errors.New("no affect row")
	}

	return res.LastInsertId()
}

// *******************************
// internal help function
// *******************************

func (sql *Sql) getLimit() string {
	if sql.limit == "" {
		return ""
	}
	return " limit " + sql.limit + " "
}

func (sql *Sql) getOffset() string {
	if sql.offset == "" {
		return ""
	}
	return " offset " + sql.offset + " "
}

func (sql *Sql) getOrderBy() string {
	if sql.order == "" {
		return ""
	}
	return " order by " + sql.order + " "
}

func (sql *Sql) getJoins() string {
	if len(sql.leftjoins) == 0 {
		return ""
	}
	joins := ""
	for _, join := range sql.leftjoins {
		joins += " left join " + join.table + " on " + join.fieldA + " " + join.operation + " " + join.fieldB + " "
	}
	return joins
}

func (sql *Sql) getFields() string {
	if len(sql.fields) == 0 {
		return "*"
	}
	if sql.fields[0] == "count(*)" {
		return "count(*)"
	}
	fields := ""
	if len(sql.leftjoins) == 0 {
		for _, field := range sql.fields {
			fields += "`" + field + "`,"
		}
	} else {
		for _, field := range sql.fields {
			arr := strings.Split(field, ".")
			if len(arr) > 1 {
				fields += arr[0] + ".`" + arr[1] + "`,"
			} else {
				fields += "`" + field + "`,"
			}
		}
	}
	return fields[:len(fields)-1]
}

func (sql *Sql) getWheres() string {
	if len(sql.wheres) == 0 {
		if sql.whereRaw != "" {
			return " where " + sql.whereRaw
		}
		return ""
	}
	wheres := " where "
	for _, where := range sql.wheres {
		wheres += where.field + " " + where.operation + " " + where.qmark + " and "
	}

	if sql.whereRaw != "" {
		return wheres + sql.whereRaw
	} else {
		return wheres[:len(wheres)-5]
	}
}

func (sql *Sql) prepareUpdate(values H) {
	fields := ""
	args := make([]interface{}, 0)

	if len(values) != 0 {

		for key, value := range values {
			fields += "`" + key + "` = ?, "
			args = append(args, value)
		}

		if len(sql.updateRaw) == 0 {
			fields = fields[:len(fields)-2]
		} else {
			for i := 0; i < len(sql.updateRaw); i++ {
				if i == len(sql.updateRaw)-1 {
					fields += sql.updateRaw[i].expression + " "
				} else {
					fields += sql.updateRaw[i].expression + ","
				}
				args = append(args, sql.updateRaw[i].args...)
			}
		}

		sql.args = append(args, sql.args...)
	} else {
		if len(sql.updateRaw) == 0 {
			panic("prepareUpdate: wrong parameter")
		} else {
			for i := 0; i < len(sql.updateRaw); i++ {
				if i == len(sql.updateRaw)-1 {
					fields += sql.updateRaw[i].expression + " "
				} else {
					fields += sql.updateRaw[i].expression + ","
				}
				args = append(args, sql.updateRaw[i].args...)
			}
		}
		sql.args = append(args, sql.args...)
	}

	sql.statement = "update " + sql.table + " set " + fields + sql.getWheres()
}

func (sql *Sql) prepareInsert(values H) {
	fields := "("
	quesMark := "("

	for key, value := range values {
		fields += "`" + key + "`,"
		quesMark += "?,"
		sql.args = append(sql.args, value)
	}
	fields = fields[:len(fields)-1] + ")"
	quesMark = quesMark[:len(quesMark)-1] + ")"

	sql.statement = "insert into " + sql.table + fields + " values " + quesMark
}

func (sql *Sql) empty() *Sql {
	sql.fields = make([]string, 0)
	sql.args = make([]interface{}, 0)
	sql.table = ""
	sql.wheres = make([]Where, 0)
	sql.leftjoins = make([]Join, 0)
	return sql
}

func (sql *Sql) log() {
	if config.GetEnv().SQL_LOG {
		logger.Info(logger.E{
			Info:logger.M{
				"statement": sql.statement,
				"args": sql.args,
			},
		})
	}
}
