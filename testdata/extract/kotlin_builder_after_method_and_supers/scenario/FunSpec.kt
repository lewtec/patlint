@OptIn(ExperimentalKotlinPoetApi::class)
public class FunSpec
private constructor(
  builder: Builder,
) :
  Taggable by tagMap,
  Documentable {
  public fun emit() {}
  public val receiverKdoc: Int = 0
  public class Builder internal constructor(internal val name: String) :
    Taggable by tagMap {
    internal var receiverKdoc = 0
  }
}
