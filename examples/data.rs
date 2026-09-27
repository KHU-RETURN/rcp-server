use std::io::{self, BufRead, Write};

fn main() {
    let stdin = io::stdin();
    let mut lines = stdin.lock().lines();
    let _event = lines.next().expect("event").expect("read event");

    println!("{}", r#"{"$rcp":"db","op":"put","collection":"visits","key":"count","value":1}"#);
    io::stdout().flush().expect("flush put");
    let put = lines.next().expect("put reply").expect("read put reply");
    if !put.contains("\"ok\":true") { panic!("put failed: {}", put); }

    println!("{}", r#"{"$rcp":"db","op":"get","collection":"visits","key":"count"}"#);
    io::stdout().flush().expect("flush get");
    let get = lines.next().expect("get reply").expect("read get reply");
    if !get.contains("\"value\":1") { panic!("get failed: {}", get); }

    println!("{}", r#"{"statusCode":200,"headers":{"content-type":"application/json"},"body":"{\"count\":1}"}"#);
}
