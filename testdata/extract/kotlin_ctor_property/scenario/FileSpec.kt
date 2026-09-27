public class FileSpec
private constructor(builder: Builder) {
  public class Builder
  internal constructor(
    public val isScript: Boolean,
  ) {
    public fun build(): FileSpec = FileSpec(this)
  }
}
