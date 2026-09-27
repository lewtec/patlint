pub const TGA = struct {
    pub const EncoderOptions = struct {
        job_time: struct { hours: u16 = 0 } = .{},
    };
};
const TargaRLEDecoder = struct {
    pub fn deinit(self: TargaRLEDecoder) void { _ = self; }
};
