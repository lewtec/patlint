public class TypeVariableName
private constructor(public val name: String) {
  override fun copy(nullable: Boolean): TypeVariableName = this
}
