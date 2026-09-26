// Package jvm is the JVM language family platform (family id "jvm").
//
// Surfaces (honest language ids, separate drivers/grammars):
//   - java   — implemented (pkg/ingest/jvm/java), extensions .java
//   - scala  — implemented (pkg/ingest/jvm/scala), extensions .scala
//   - kotlin — implemented (pkg/ingest/jvm/kotlin), extensions .kt .kts
//
// Shared lattice: package directory is the module; declaration grain is
// types/members; file is layout within a package.
//
// Blank-import this package before registering surfaces, or import for Family.
package jvm
