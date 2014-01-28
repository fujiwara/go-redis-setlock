go-redis-setlock: main.go
	go build

test: go-redis-setlock
	prove t

clean:
	rm go-redis-setlock index.html

binary: main.go
	script/build.sh

index.html: README.md
	curl -s -H"Content-Type: text/x-markdown" -X POST --data-binary @README.md https://api.github.com/markdown/raw > index.html

release: index.html
	git checkout gh-pages
	git merge master
	script/build.sh
	git add bin && git commit -m "release binary" && git push origin gh-pages
	git checkout -

