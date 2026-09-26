package pattern_test

import (
	_ "github.com/lewtec/patlint/pkg/ingest/go"
	_ "github.com/lewtec/patlint/pkg/ingest/go/templ"
	_ "github.com/lewtec/patlint/pkg/ingest/html"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/java"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/kotlin"
	_ "github.com/lewtec/patlint/pkg/ingest/jvm/scala"
	_ "github.com/lewtec/patlint/pkg/ingest/python"
	_ "github.com/lewtec/patlint/pkg/sitter/ccgo"
)
