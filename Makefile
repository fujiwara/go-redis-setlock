all:
	@echo "do nothing"

binary: main.go
	script/build.sh

index.html: README.md
	curl -s -H"Content-Type: text/x-markdown" -X POST --data-binary @README.md https://api.github.com/markdown/raw > index.html

release: index.html
	git checkout gh-pages
	script/build.sh
	git merge master
	git commit -m "release binary"
	git push origin gh-pages
	git checkout -
