struct Searcher {}
impl Searcher {
    pub fn find(&self) {}
    pub fn is_match(&self) {}
    pub fn find_in<B: AsRef<[u8]>>(&self) {}
}
impl Display for Searcher {
    fn fmt(&self) {}
}
impl<V: Vector> Fat<V, 1> {
    unsafe fn candidate(&self) {}
}
impl<T> Box {
    fn wrap(&self) {}
}
