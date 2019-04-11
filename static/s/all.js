(function() {
    function $(selctor) {
        return document.querySelector(selctor)
    }
    
    function loadData() {
        const content = $('#content').value
        const results = { content }
        const queries = document.location.search.slice(1).split('&')
        queries.forEach(item => {
            const [key, value] = item.split('=')
            results[decodeURIComponent(key)] = decodeURIComponent(value)
        })

        return results
    }

    function addComment() {
        const values = loadData()
        const headers = new Headers()
        headers.set('Content-Type', 'application/json')
        fetch('/comments', {
            method: 'POST',
            headers,
            body: JSON.stringify(values)
        })
        .then(res => res.json())
        .then(data => {
            $('#result').innerHTML += mkCommentDOM({
                ...data,
                image: $('#user-image').src,
                name: $('#username').innerText,
            })
            resizeWindow()
            $('#content').value = ""
        }).catch(err => {
            // message
        })
    }

    function requestComments() {
        const values = loadData()
        const { hostname, target } = values
        fetch(`/comments?hostname=${hostname}&target=${target}`)
            .then(res => res.json())
            .then(items => {
                const commentList = $('#result')
                const comments = items.map(mkCommentDOM).join('')
                commentList.innerHTML += comments
                resizeWindow()
            })
            .catch(err => {
                // message
            }) // do nothing
    }

    function mkCommentDOM(comment) {
        const { content, name, image } = comment
        const commentElement = `
        <div class="g comment">
            <div class="g-c-side">
                <figure>
                    <img src="${image}" height="64" alt="${name}" />
                </figure>
            </div>
            <div class="g-c-fill">
                <span class="comment-username">${name}</span>
                <div class="comment-content">${content}</div>
            </div>
        </div>
        `

        return commentElement
    }

    function resizeWindow() {
        const height = document.body.offsetHeight
        window.parent.postMessage(height + 300, '*')
    }

    document.addEventListener('DOMContentLoaded', function() {
        requestComments()
        $('#submit').addEventListener('click', event => {
            event.preventDefault()
            if ($('#form').reportValidity()) {
                addComment()
            }
        })
    })
})()