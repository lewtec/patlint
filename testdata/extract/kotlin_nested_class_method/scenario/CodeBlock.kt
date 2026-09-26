public class CodeBlock
private constructor(internal val formatParts: List<String>) {
  public class Builder {
    private fun argToString(o: Any?) = o?.toString()
    public fun build(): CodeBlock = CodeBlock(formatParts)
  }
}
