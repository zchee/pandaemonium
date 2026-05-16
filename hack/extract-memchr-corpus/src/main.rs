// Copyright 2026 The pandaemonium Authors.
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

//! Golden-corpus extraction shim for the Go internal/memchr port.
//!
//! Enumerates a deterministic table of fixtures across:
//!   * lengths covering tail-handler boundaries (0..=1024)
//!   * synthetic haystack patterns (zeros, ones, alternating, sequential)
//!   * planted-needle positions (boundary indices)
//!   * single-/pair-/triple-needle routines
//!
//! For each fixture the upstream `memchr` crate (pinned at =2.7.4) is the
//! oracle: its return value becomes the JSON `want` field. The Go test
//! `TestGoldenCorpus` reads the JSON, base64-decodes each haystack, calls
//! the dispatched Go impl, and asserts byte-exact parity. See
//! `internal/memchr/golden_test.go`.
//!
//! Regen:
//!   (cd hack/extract-memchr-corpus && cargo run --release) \
//!       > internal/memchr/testdata/golden_corpus.json

use std::io::{self, BufWriter, Write};

use base64::{engine::general_purpose::STANDARD, Engine};
use memchr::{memchr, memchr2, memchr3, memrchr, memrchr2, memrchr3};
use serde::Serialize;

#[derive(Serialize)]
struct Fixture {
    routine: &'static str,
    needles: Vec<u8>,
    haystack_b64: String,
    want: isize,
}

fn pattern_zeros(len: usize) -> Vec<u8> {
    vec![0u8; len]
}

fn pattern_ones(len: usize) -> Vec<u8> {
    vec![0xffu8; len]
}

fn pattern_alt(len: usize) -> Vec<u8> {
    (0..len)
        .map(|i| if i & 1 == 0 { 0x55 } else { 0xaa })
        .collect()
}

fn pattern_seq(len: usize) -> Vec<u8> {
    (0..len).map(|i| (i & 0x7f) as u8).collect()
}

fn want(opt: Option<usize>) -> isize {
    opt.map(|i| i as isize).unwrap_or(-1)
}

fn push_single(out: &mut Vec<Fixture>, hay: &[u8], needle: u8) {
    let hayb64 = STANDARD.encode(hay);
    out.push(Fixture {
        routine: "memchr",
        needles: vec![needle],
        haystack_b64: hayb64.clone(),
        want: want(memchr(needle, hay)),
    });
    out.push(Fixture {
        routine: "memrchr",
        needles: vec![needle],
        haystack_b64: hayb64,
        want: want(memrchr(needle, hay)),
    });
}

fn push_pair(out: &mut Vec<Fixture>, hay: &[u8], n1: u8, n2: u8) {
    let hayb64 = STANDARD.encode(hay);
    out.push(Fixture {
        routine: "memchr2",
        needles: vec![n1, n2],
        haystack_b64: hayb64.clone(),
        want: want(memchr2(n1, n2, hay)),
    });
    out.push(Fixture {
        routine: "memrchr2",
        needles: vec![n1, n2],
        haystack_b64: hayb64,
        want: want(memrchr2(n1, n2, hay)),
    });
}

fn push_triple(out: &mut Vec<Fixture>, hay: &[u8], n1: u8, n2: u8, n3: u8) {
    let hayb64 = STANDARD.encode(hay);
    out.push(Fixture {
        routine: "memchr3",
        needles: vec![n1, n2, n3],
        haystack_b64: hayb64.clone(),
        want: want(memchr3(n1, n2, n3, hay)),
    });
    out.push(Fixture {
        routine: "memrchr3",
        needles: vec![n1, n2, n3],
        haystack_b64: hayb64,
        want: want(memrchr3(n1, n2, n3, hay)),
    });
}

fn plant_positions(len: usize) -> Vec<usize> {
    match len {
        0 => Vec::new(),
        1 => vec![0],
        2..=15 => vec![0, len - 1],
        _ => {
            let mut v = vec![0, 1, 7, 8, len / 2, len.saturating_sub(9), len - 1];
            v.sort_unstable();
            v.dedup();
            v
        }
    }
}

fn main() -> io::Result<()> {
    let lens: &[usize] = &[
        0, 1, 7, 8, 9, 15, 16, 17, 31, 32, 33, 63, 64, 65, 127, 128, 255, 256, 1024,
    ];
    let patterns: &[fn(usize) -> Vec<u8>] = &[
        pattern_zeros,
        pattern_ones,
        pattern_alt,
        pattern_seq,
    ];
    let singles: &[u8] = &[0x00, 0x01, 0x55, 0x7f, 0x80, 0xaa, 0xfe, 0xff];
    let pairs: &[(u8, u8)] = &[
        (0x00, 0xff),
        (0x55, 0xaa),
        (0x7f, 0x80),
        (0x01, 0xfe),
        (0xC3, 0xD4),
    ];
    let triples: &[(u8, u8, u8)] = &[
        (0x00, 0x55, 0xff),
        (0x01, 0x7f, 0x80),
        (0xC3, 0xD4, 0xE5),
        (0xaa, 0xbb, 0xcc),
    ];

    let mut out: Vec<Fixture> = Vec::new();

    // Single-needle: pattern haystacks × every needle value, plus planted
    // sentinel (0xC3) at every boundary position to exercise FIRST/LAST
    // asymmetry between memchr and memrchr.
    for &len in lens {
        for &gen in patterns {
            let hay = gen(len);
            for &n in singles {
                push_single(&mut out, &hay, n);
            }
            for pos in plant_positions(len) {
                let mut planted = hay.clone();
                planted[pos] = 0xC3;
                push_single(&mut out, &planted, 0xC3);
                // Two-instance plant distinguishes memchr from memrchr.
                if pos < len - 1 {
                    let mut planted2 = planted.clone();
                    planted2[len - 1] = 0xC3;
                    push_single(&mut out, &planted2, 0xC3);
                }
            }
        }
    }

    // Pair-needle: pattern haystacks × every pair.
    for &len in lens {
        for &gen in patterns {
            let hay = gen(len);
            for &(n1, n2) in pairs {
                push_pair(&mut out, &hay, n1, n2);
            }
            // Planted-pair fixture: 0xC3 at pos[0], 0xD4 at pos[last] to
            // force memchr2 / memrchr2 onto different offsets.
            for &(n1, n2) in &[(0xC3u8, 0xD4u8)] {
                for pos in plant_positions(len) {
                    let mut planted = hay.clone();
                    planted[pos] = n1;
                    if len > 1 && pos != len - 1 {
                        planted[len - 1] = n2;
                    }
                    push_pair(&mut out, &planted, n1, n2);
                }
            }
        }
    }

    // Triple-needle: pattern haystacks × every triple.
    for &len in lens {
        for &gen in patterns {
            let hay = gen(len);
            for &(n1, n2, n3) in triples {
                push_triple(&mut out, &hay, n1, n2, n3);
            }
            for &(n1, n2, n3) in &[(0xC3u8, 0xD4u8, 0xE5u8)] {
                for pos in plant_positions(len) {
                    let mut planted = hay.clone();
                    planted[pos] = n1;
                    if len > 1 {
                        if pos > 0 {
                            planted[pos - 1] = n2;
                        }
                        if pos != len - 1 {
                            planted[len - 1] = n3;
                        }
                    }
                    push_triple(&mut out, &planted, n1, n2, n3);
                }
            }
        }
    }

    let stdout = io::stdout().lock();
    let mut w = BufWriter::new(stdout);
    serde_json::to_writer_pretty(&mut w, &out).map_err(io::Error::other)?;
    w.write_all(b"\n")?;
    Ok(())
}
