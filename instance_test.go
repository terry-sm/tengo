package tengo

import (
	"database/sql"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestNewInstance(t *testing.T) {
	assertError := func(driver, dsn string) {
		instance, err := NewInstance(driver, dsn)
		if instance != nil || err == nil {
			t.Errorf("Expected NewInstance(\"%s\", \"%s\") to return nil,err; instead found %v, %v", driver, dsn, instance, err)
		}
	}
	assertError("btrieve", "username:password@tcp(some.host)/dbname?param=value")
	assertError("", "username:password@tcp(some.host:1234)/dbname?param=value")
	assertError("mysql", "username:password@tcp(some.host:1234) i like zebras")

	assertInstance := func(dsn string, expectedInstance Instance) {
		expectedInstance.connectionPool = make(map[string]*sqlx.DB)
		instance, err := NewInstance("mysql", dsn)
		if err != nil {
			t.Fatalf("Unexpectedly received error %s from NewInstance(\"mysql\", \"%s\")", err, dsn)
		}
		expectedInstance.RWMutex = instance.RWMutex // cheat to satisfy DeepEqual
		if !reflect.DeepEqual(expectedInstance, *instance) {
			t.Errorf("NewInstance(\"mysql\", \"%s\"): Returned instance %#v does not match expected instance %#v", dsn, *instance, expectedInstance)
		}
	}

	dsn := "username:password@tcp(some.host:1234)/dbname"
	expected := Instance{
		BaseDSN:       "username:password@tcp(some.host:1234)/",
		Driver:        "mysql",
		User:          "username",
		Password:      "password",
		Host:          "some.host",
		Port:          1234,
		defaultParams: map[string]string{},
	}
	assertInstance(dsn, expected)

	dsn = "username:password@tcp(1.2.3.4:3306)/?param1=value1&readTimeout=5s&interpolateParams=0"
	expected = Instance{
		BaseDSN:  "username:password@tcp(1.2.3.4:3306)/",
		Driver:   "mysql",
		User:     "username",
		Password: "password",
		Host:     "1.2.3.4",
		Port:     3306,
		defaultParams: map[string]string{
			"param1":            "value1",
			"readTimeout":       "5s",
			"interpolateParams": "0",
		},
	}
	assertInstance(dsn, expected)

	dsn = "root@unix(/var/lib/mysql/mysql.sock)/dbname?param1=value1"
	expected = Instance{
		BaseDSN:    "root@unix(/var/lib/mysql/mysql.sock)/",
		Driver:     "mysql",
		User:       "root",
		Host:       "localhost",
		SocketPath: "/var/lib/mysql/mysql.sock",
		defaultParams: map[string]string{
			"param1": "value1",
		},
	}
	assertInstance(dsn, expected)
}

func TestInstanceBuildParamString(t *testing.T) {
	assertParamString := func(defaultOptions, addOptions, expectOptions string) {
		dsn := "username:password@tcp(1.2.3.4:3306)/"
		if defaultOptions != "" {
			dsn += "?" + defaultOptions
		}
		instance, err := NewInstance("mysql", dsn)
		if err != nil {
			t.Fatalf("NewInstance(\"mysql\", \"%s\") returned error: %s", dsn, err)
		}

		// can't compare strings directly since order may be different
		result := instance.buildParamString(addOptions)
		parsedResult, err := url.ParseQuery(result)
		if err != nil {
			t.Fatalf("url.ParseQuery(\"%s\") returned error: %s", result, err)
		}
		parsedExpected, err := url.ParseQuery(expectOptions)
		if err != nil {
			t.Fatalf("url.ParseQuery(\"%s\") returned error: %s", expectOptions, err)
		}
		if !reflect.DeepEqual(parsedResult, parsedExpected) {
			t.Errorf("Expected param map %v, instead found %v", parsedExpected, parsedResult)
		}
	}

	assertParamString("", "", "")
	assertParamString("param1=value1", "", "param1=value1")
	assertParamString("", "param1=value1", "param1=value1")
	assertParamString("param1=value1", "param1=value1", "param1=value1")
	assertParamString("param1=value1", "param1=hello", "param1=hello")
	assertParamString("param1=value1&readTimeout=5s&interpolateParams=0", "param2=value2", "param1=value1&readTimeout=5s&interpolateParams=0&param2=value2")
	assertParamString("param1=value1&readTimeout=5s&interpolateParams=0", "param1=value3", "param1=value3&readTimeout=5s&interpolateParams=0")
}

func (s TengoIntegrationSuite) TestInstanceConnect(t *testing.T) {
	// Connecting to invalid schema should return an error
	db, err := s.d.Connect("does-not-exist", "")
	if err == nil {
		t.Error("err is unexpectedly nil")
	} else if db != nil {
		t.Error("db is unexpectedly non-nil")
	}

	// Connecting without specifying a default schema should be successful
	db, err = s.d.Connect("", "")
	if err != nil {
		t.Errorf("Unexpected connection error: %s", err)
	} else if db == nil {
		t.Error("db is unexpectedly nil")
	}

	// Connecting again with same schema and params should return the existing connection pool
	db2, err := s.d.Connect("", "")
	if err != nil {
		t.Errorf("Unexpected connection error: %s", err)
	} else if db2 != db {
		t.Errorf("Expected same DB pool to be returned from identical Connect call; instead db=%v and db2=%v", db, db2)
	}

	// Connecting again with different schema should return a different connection pool
	db3, err := s.d.Connect("information_schema", "")
	if err != nil {
		t.Errorf("Unexpected connection error: %s", err)
	} else if db3 == db {
		t.Error("Expected different DB pool to be returned from Connect with different default db; instead was same")
	}

	// Connecting again with different params should return a different connection pool
	db4, err := s.d.Connect("information_schema", "foreign_key_checks=0")
	if err != nil {
		t.Errorf("Unexpected connection error: %s", err)
	} else if db4 == db || db4 == db3 {
		t.Error("Expected different DB pool to be returned from Connect with different params; instead was same")
	}
}

func (s TengoIntegrationSuite) TestInstanceFlavorVersion(t *testing.T) {
	type parsed struct {
		flavor Flavor
		major  int
		minor  int
	}
	imageToParsed := map[string]parsed{
		"mysql:5.5":    {FlavorMySQL, 5, 5},
		"mysql:5.6":    {FlavorMySQL, 5, 6},
		"mysql:5.7":    {FlavorMySQL, 5, 7},
		"mysql:8.0":    {FlavorMySQL, 8, 0},
		"percona:5.5":  {FlavorPercona, 5, 5},
		"percona:5.6":  {FlavorPercona, 5, 6},
		"percona:5.7":  {FlavorPercona, 5, 7},
		"percona:8.0":  {FlavorPercona, 8, 0},
		"mariadb:10.1": {FlavorMariaDB, 10, 1},
		"mariadb:10.2": {FlavorMariaDB, 10, 2},
		"mariadb:10.3": {FlavorMariaDB, 10, 3},
	}

	var expected parsed
	if result, ok := imageToParsed[s.d.Image]; ok {
		expected = result
	} else {
		for image, result := range imageToParsed {
			tokens := strings.SplitN(image, ":", 2)
			if len(tokens) < 2 {
				continue
			}
			repository, tag := tokens[0], tokens[1]
			if strings.Contains(s.d.Image, repository) && strings.Contains(s.d.Image, tag) {
				expected = result
				break
			}
		}
	}
	if expected.flavor == FlavorUnknown {
		t.Skip("No image map defined for", s.d.Image)
	}
	if actualFlavor := s.d.Flavor(); actualFlavor != expected.flavor {
		t.Errorf("Expected image=%s to yield flavor=%s, instead found %s", s.d.Image, expected.flavor, actualFlavor)
	}
	if actualMajor, actualMinor, _ := s.d.Version(); actualMajor != expected.major || actualMinor != expected.minor {
		t.Errorf("Expected image=%s to yield major=%d minor=%d, instead found major=%d minor=%d", s.d.Image, expected.major, expected.minor, actualMajor, actualMinor)
	}
}

func (s TengoIntegrationSuite) TestInstanceSchemas(t *testing.T) {
	// Currently at least 4 schemas in testdata/integration.sql
	schemas, err := s.d.Schemas()
	if err != nil || len(schemas) < 4 {
		t.Errorf("Expected at least 4 schemas, instead found %d, err=%s", len(schemas), err)
	}

	// Ensure SchemasByName is returning the same set of schemas
	byName, err := s.d.SchemasByName()
	if err != nil {
		t.Errorf("SchemasByName returned error: %s", err)
	} else if len(byName) != len(schemas) {
		t.Errorf("len(byName) != len(schemas): %d vs %d", len(byName), len(schemas))
	}
	seen := make(map[string]bool, len(byName))
	for _, schema := range schemas {
		if seen[schema.Name] {
			t.Errorf("Schema %s returned multiple times from call to instance.Schemas", schema.Name)
		}
		seen[schema.Name] = true
		if !reflect.DeepEqual(schema, byName[schema.Name]) {
			t.Errorf("Mismatch for schema %s between Schemas and SchemasByName", schema.Name)
		}
		if schema2, err := s.d.Schema(schema.Name); err != nil || !reflect.DeepEqual(schema2, schema) {
			t.Errorf("Mismatch for schema %s vs instance.Schema(%s); error=%s", schema.Name, schema.Name, err)
		}
		if has, err := s.d.HasSchema(schema.Name); !has || err != nil {
			t.Errorf("Expected HasSchema(%s)==true, instead found false", schema.Name)
		}
	}

	// Test SchemasByName with args
	byName, err = s.d.SchemasByName("testcharset", "doesnt_exist", "testcharcoll")
	if err != nil {
		t.Errorf("SchemasByName returned error: %s", err)
	}
	if len(byName) != 2 {
		t.Errorf("SchemasByName returned wrong number of results; expected 2, found %d", len(byName))
	}
	for name, schema := range byName {
		if name != schema.Name || (name != "testcharset" && name != "testcharcoll") {
			t.Errorf("SchemasByName returned mismatching schema: key=%s, name=%s", name, schema.Name)
		}
	}

	// Test negative responses
	if has, err := s.d.HasSchema("doesnt_exist"); has || err != nil {
		t.Error("HasSchema(doesnt_exist) unexpectedly returning true")
	}
	if schema, err := s.d.Schema("doesnt_exist"); schema != nil || err != sql.ErrNoRows {
		t.Errorf("Expected Schema(doesnt_exist) to return nil,sql.ErrNoRows; instead found %v,%s", schema, err)
	}
}

func (s TengoIntegrationSuite) TestInstanceShowCreateTable(t *testing.T) {
	t1create, err1 := s.d.ShowCreateTable("testing", "actor")
	t2create, err2 := s.d.ShowCreateTable("testing", "actor_in_film")
	if err1 != nil || err2 != nil || t1create == "" || t2create == "" {
		t.Fatalf("Unable to obtain SHOW CREATE TABLE output: err1=%s, err2=%s", err1, err2)
	}

	t1expected := aTable(1)
	if s.d.IsNewMariaFormat() {
		t1expected.CreateStatement = strings.Replace(t1expected.CreateStatement, "CURRENT_TIMESTAMP", "current_timestamp", -1)
		t1expected.CreateStatement = strings.Replace(t1expected.CreateStatement, "tinyint(1) NOT NULL DEFAULT '1'", "tinyint(1) NOT NULL DEFAULT 1", 1)
	}

	if t1create != t1expected.CreateStatement {
		t.Errorf("Mismatch for SHOW CREATE TABLE\nActual return from %s:\n%s\n----------\nExpected output: %s", s.d.Image, t1create, t1expected.CreateStatement)
	}

	t2expected := anotherTable()
	if t2create != t2expected.CreateStatement {
		t.Errorf("Mismatch for SHOW CREATE TABLE\nActual return from %s:\n%s\n----------\nExpected output: %s", s.d.Image, t2create, t2expected.CreateStatement)
	}

	// Test nonexistent table
	t3create, err3 := s.d.ShowCreateTable("testing", "doesnt_exist")
	if t3create != "" || err3 == nil {
		t.Errorf("Expected ShowCreateTable on invalid table to return empty string and error, instead err=%s, output=%s", err3, t3create)
	}
}

func (s TengoIntegrationSuite) TestInstanceTableSize(t *testing.T) {
	size, err := s.d.TableSize("testing", "has_rows")
	if err != nil {
		t.Errorf("Error from TableSize: %s", err)
	} else if size < 1 {
		t.Errorf("TableSize returned a non-positive result: %d", size)
	}

	// Test nonexistent table
	size, err = s.d.TableSize("testing", "doesnt_exist")
	if size > 0 || err == nil {
		t.Errorf("Expected TableSize to return 0 size and non-nil err for missing table, instead size=%d and err=%s", size, err)
	}
}

func (s TengoIntegrationSuite) TestInstanceTableHasRows(t *testing.T) {
	if hasRows, err := s.d.TableHasRows("testing", "has_rows"); err != nil {
		t.Errorf("Error from TableHasRows: %s", err)
	} else if !hasRows {
		t.Error("Expected TableHasRows to return true for has_rows, instead returned false")
	}

	if hasRows, err := s.d.TableHasRows("testing", "no_rows"); err != nil {
		t.Errorf("Error from TableHasRows: %s", err)
	} else if hasRows {
		t.Error("Expected TableHasRows to return false for no_rows, instead returned true")
	}

	// Test nonexistent table
	if _, err := s.d.TableHasRows("testing", "doesnt_exist"); err == nil {
		t.Error("Expected TableHasRows to return error for nonexistent table, but it did not")
	}
}

func (s TengoIntegrationSuite) TestInstanceCreateSchema(t *testing.T) {
	_, err := s.d.CreateSchema("foobar", "utf8mb4", "utf8mb4_unicode_ci")
	if err != nil {
		t.Fatalf("CreateSchema returned unexpected error: %s", err)
	}
	if refetch, err := s.d.Schema("foobar"); err != nil {
		t.Errorf("Unable to fetch newly created schema: %s", err)
	} else if refetch.CharSet != "utf8mb4" || refetch.Collation != "utf8mb4_unicode_ci" {
		t.Errorf("Unexpected charset or collation on refetched schema: %+v", refetch)
	}

	// Ensure creation of duplicate schema fails with error
	if _, err := s.d.CreateSchema("foobar", "utf8mb4", "utf8mb4_unicode_ci"); err == nil {
		t.Error("Expected creation of duplicate schema to return an error, but it did not")
	}

	// Creation of schema without specifying charset and collation should use
	// instance defaults
	defCharSet, defCollation, err := s.d.DefaultCharSetAndCollation()
	if err != nil {
		t.Fatalf("Unable to obtain instance default charset and collation")
	}
	if schema, err := s.d.CreateSchema("barfoo", "", ""); err != nil {
		t.Errorf("Failed to create schema with default charset and collation: %s", err)
	} else if schema.CharSet != defCharSet || schema.Collation != defCollation {
		t.Errorf("Expected charset/collation to be %s/%s, instead found %s/%s", defCharSet, defCollation, schema.CharSet, schema.Collation)
	}
}

func (s TengoIntegrationSuite) TestInstanceDropSchema(t *testing.T) {
	// Dropping a schema with non-empty tables when onlyIfEmpty==true should fail
	if err := s.d.DropSchema("testing", true); err == nil {
		t.Error("Expected dropping a schema with tables to fail when onlyIfEmpty==true, but it did not")
	}

	// Dropping a schema without tables when onlyIfEmpty==true should succeed
	if err := s.d.DropSchema("testcollate", true); err != nil {
		t.Errorf("Expected dropping a schema without tables to succeed when onlyIfEmpty==true, but error=%s", err)
	}

	// Dropping a schema with only empty tables when onlyIfEmpty==true should succeed
	if err := s.d.DropSchema("testcharcoll", true); err != nil {
		t.Errorf("Expected dropping a schema with only empty tables to succeed when onlyIfEmpty==true, but error=%s", err)
	}

	// Dropping a schema with non-empty tables when onlyIfEmpty==false should succeed
	if err := s.d.DropSchema("testing", false); err != nil {
		t.Errorf("Expected dropping a schema with tables to succeed when onlyIfEmpty==false, but error=%s", err)
	}

	// Dropping a schema that doesn't exist should fail
	if err := s.d.DropSchema("testing", false); err == nil {
		t.Error("Expected dropping a nonexistent schema to fail, but error was nil")
	}
}

func (s TengoIntegrationSuite) TestInstanceAlterSchema(t *testing.T) {
	assertNoError := func(schemaName, newCharSet, newCollation, expectCharSet, expectCollation string) {
		t.Helper()
		if err := s.d.AlterSchema(schemaName, newCharSet, newCollation); err != nil {
			t.Errorf("Expected alter of %s to (%s,%s) would not error, but returned %s", schemaName, newCharSet, newCollation, err)
		} else {
			schema, err := s.d.Schema(schemaName)
			if err != nil {
				t.Fatalf("Unexpected error fetching schema: %s", err)
			}
			if schema.CharSet != expectCharSet {
				t.Errorf("Expected post-alter charset to be %s, instead found %s", expectCharSet, schema.CharSet)
			}
			if schema.Collation != expectCollation {
				t.Errorf("Expected post-alter collation to be %s, instead found %s", expectCollation, schema.Collation)
			}
		}
	}
	assertError := func(schemaName, newCharSet, newCollation string) {
		t.Helper()
		if err := s.d.AlterSchema(schemaName, newCharSet, newCollation); err == nil {
			t.Errorf("Expected alter of %s to (%s,%s) would return error, but returned nil instead", schemaName, newCharSet, newCollation)
		}
	}

	instCharSet, instCollation, err := s.d.DefaultCharSetAndCollation()
	if err != nil {
		t.Fatalf("Unable to fetch instance default charset and collation: %s", err)
	}

	// `testing` has instance-default charset and collation
	// `testcharset` has utf8mb4 charset with its default collation (utf8mb4_general_ci)
	// `testcharcoll` has utf8mb4 with utf8mb4_unicode_ci

	// Test no-op conditions
	assertNoError("testing", "", "", instCharSet, instCollation)
	assertNoError("testcharset", "utf8mb4", "", "utf8mb4", "utf8mb4_general_ci")
	assertNoError("testcharset", "", "utf8mb4_general_ci", "utf8mb4", "utf8mb4_general_ci")
	assertNoError("testcharcoll", "utf8mb4", "utf8mb4_unicode_ci", "utf8mb4", "utf8mb4_unicode_ci")

	// Test known error conditions
	assertError("testing", "badcharset", "badcollation")    // charset and collation are invalid
	assertError("testcharset", "utf8", "latin1_swedish_ci") // charset and collation do not match
	assertError("nonexistent", "utf8mb4", "")               // schema does not actually exist in instance

	// Test successful alters
	assertNoError("testcharset", "", "utf8mb4_unicode_ci", "utf8mb4", "utf8mb4_unicode_ci")
	assertNoError("testcharcoll", "latin1", "", "latin1", "latin1_swedish_ci")
	assertNoError("testing", "utf8mb4", "utf8mb4_general_ci", "utf8mb4", "utf8mb4_general_ci")
}

func (s TengoIntegrationSuite) TestInstanceSchemaIntrospection(t *testing.T) {
	// Ensure our unit test fixtures and integration test fixtures match
	schema, aTableFromDB := s.GetSchemaAndTable(t, "testing", "actor")
	aTableFromUnit := aTable(1)
	if s.d.IsNewMariaFormat() {
		aTableFromUnit.CreateStatement = strings.Replace(aTableFromUnit.CreateStatement, "CURRENT_TIMESTAMP", "current_timestamp", -1)
		aTableFromUnit.CreateStatement = strings.Replace(aTableFromUnit.CreateStatement, "tinyint(1) NOT NULL DEFAULT '1'", "tinyint(1) NOT NULL DEFAULT 1", 1)
	}
	clauses, supported := aTableFromDB.Diff(&aTableFromUnit)
	if !supported {
		t.Error("Diff unexpectedly not supported for testing.actor")
	} else if len(clauses) > 0 {
		t.Errorf("Diff of testing.actor unexpectedly found %d clauses; expected 0", len(clauses))
	}
	aTableFromDB = s.GetTable(t, "testing", "actor_in_film")
	aTableFromUnit = anotherTable()
	clauses, supported = aTableFromDB.Diff(&aTableFromUnit)
	if !supported {
		t.Error("Diff unexpectedly not supported for testing.actor_in_film")
	} else if len(clauses) > 0 {
		t.Errorf("Diff of testing.actor_in_film unexpectedly found %d clauses; expected 0", len(clauses))
	}

	// ensure tables are all supported (except where known not to be)
	for _, table := range schema.Tables {
		shouldBeUnsupported := (table.Name == unsupportedTable().Name)
		if table.UnsupportedDDL != shouldBeUnsupported {
			t.Errorf("Table %s: expected UnsupportedDDL==%v, instead found %v", table.Name, shouldBeUnsupported, !shouldBeUnsupported)
		}
	}
}
