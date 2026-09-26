import { readline } from "bufio"
import "strings"
import "os"
import "path"
import "fs"
import "utils"


const FOLDER_ID = "<ENTER DRIVE FOLDER ID>"
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

fn header(body, title) {
	head := `---
title: "${title}"
chapter: "${utils.parse_idx(title)}"
volume: 1
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

// convert()
// trim()
println("[Help]")
println("1) sync")
println("2) convert")
println("3) trim")
println("4) exit")
println("*) Then copy the chapters folder into your web (astro) project.")
println("")
print("input>")
for {
	input := readline() |> strings.trim_space
	print("input>")
	
	if input == "sync" {
		sync()
	}	
	else if input == "convert" {
		convert()
	}	
	else if input == "trim" {
		trim()
	}	
	else if input == "exit" {
		os.exit(0)
	}
}