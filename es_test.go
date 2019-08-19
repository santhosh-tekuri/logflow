// Copyright 2019 Santhosh Kumar Tekuri
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"reflect"
	"strings"
	"testing"
)

func Test_bulkSuccessful(t *testing.T) {
	s := `
	{
		"took": 30,
		"errors": true,
		"items": [
		   {
			  "index": {
				 "_index": "test",
				 "_type": "_doc",
				 "_id": "1",
				 "_version": 1,
				 "result": "created",
				 "_shards": {
					"total": 2,
					"successful": 11,
					"failed": 0
				 },
				 "status": 201,
				 "_seq_no" : 0,
				 "_primary_term": 1
			  }
		   },
		   {
			  "delete": {
				 "_index": "test",
				 "_type": "_doc",
				 "_id": "2",
				 "_version": 1,
				 "result": "not_found",
				 "_shards": {
					"total": 2,
					"successful": 21,
					"failed": 0
				 },
				 "status": 404,
				 "_seq_no" : 1,
				 "_primary_term" : 2
			  }
		   },
		   {
			  "create": {
				 "_index": "test",
				 "_type": "_doc",
				 "_id": "3",
				 "_version": 1,
				 "result": "created",
				 "_shards": {
					"total": 2,
					"successful": 41,
					"failed": 0
				 },
				 "status": 201,
				 "_seq_no" : 2,
				 "_primary_term" : 3
			  }
		   },
		   {
			  "update": {
				 "_index": "test",
				 "_type": "_doc",
				 "_id": "1",
				 "_version": 2,
				 "result": "updated",
				 "error": {
					"type": "mapper_parsing_exception",
					"reason": "failed to parse",
					"caused_by": {
					  "type": "json_parse_exception",
					  "reason": "Unexpected character ('\\' (code 92)): expected a valid value (number, String, array, object, 'true', 'false' or 'null')\n at [Source: org.elasticsearch.common.bytes.BytesReference$MarkSupportingStreamInputWrapper@50751cab; line: 1, column: 41]"
					}
				  },
				 "status": 200,
				 "_seq_no" : 3,
				 "_primary_term" : 4
			  }
		   }
		]
	 }
	`
	got, err := bulkSuccessful(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	want := []int{11, 21, 41, -1}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("got:", got, "want:", want)
	}
}
