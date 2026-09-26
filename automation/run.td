import { readline } from "bufio"
import "strings"
import "os"
import "path"
import "fs"
import "utils"


const FOLDER_ID = embed("folder_id.txt")
const EXE = "gdocsync"

fn sync() {
	cmd := os.exec(EXE, "sync", FOLDER_ID, "--output", "ch")
	cmd.stdout(os.stdout())
	cmd.run()
}

fn convert() {
	println("Converting...")
	cmd := os.exec(EXE, "convert", "ch", "--output", "md")
	cmd.stdout(os.stdout())
	cmd.run()
	println("Done")
}

fn get_vol(idx) {
	idx = strings.parse_float(idx, 10)
	if idx >= 0 && idx <= 19 {
		return 1
	} 
	else if idx >= 20 && idx <= 36 {
		return 2
	}
	return 3
}

fn header(body, title) {
	idx := utils.parse_idx(title)
	vol := get_vol(idx)
	// debug(vol)
	head := `---
title: "${title}"
chapter: "${idx}"
volume: ${vol}
date: "2026-09-18"
description: "${title}"
drive_id: "-"
modified: "2026-09-18T12:49:19.804Z"
---`
	
	return head + "\n" + body
}

fn trim() {
	fs.mkdir("chapters")
	for file in fs.readdir("md") {
		src := path.join("md", file.name)
		ch := fs.readfile(src) |> utils.parse_chapter
		// sysout file.name.igreen, " ", ch.title, " ", ch.date, "\n"
		// println(file.title)
		out := path.join("chapters", utils.slug(ch.title) + ".md") 
		println(out)
		fs.writefile(out, header(ch.body, ch.title))
		
	}
	
}

fn replace() {
	p := "../website/src/content/chapters"
	if fs.exists(p) {
		os.remove_all(p)
	}
	fs.mkdir(p)
	for file in fs.readdir("chapters") {
		d := path.join(p, file.name)
		fs.copy(path.join("chapters", file.name), d)
		println(d)
	}
}


// replace()
// os.exit(0)
// convert()
// trim()
println("[Help]")
println("1) sync")
println("2) convert")
println("3) trim")
println("4) replace")
println("5) exit")
println("*) auto")
println("")
print("input>")
for {
	input := readline() |> strings.trim_space
	println("")
	println("1) sync")
	println("2) convert")
	println("3) trim")
	println("4) replace")
	println("5) exit")
	println("*) auto")
	println("")
	print("input>")
	
	if input == "sync" || input == "1" {
		sync()
	}	
	else if input == "convert" || input == "2" {
		convert()
	}	
	else if input == "trim" || input == "3" {
		trim()
	}
	else if input == "replace" || input == "4" {
		replace()
	}	
	else if input == "exit" || input == "5" {
		os.exit(0)
	}	
	else if input == "auto" || input == "*" {
		sync()
		convert()
		trim()
		replace()
	}
}