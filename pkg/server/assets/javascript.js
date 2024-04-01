url = "{{url}}";
clientId = "{{clientId}}";
clientKind = "{{clientKind}}";
projectName = "{{projectName}}";
devcardName = "{{devcardName}}";
scrollPosition = 0;

/**
 * Once the website loads, we want to apply listeners and connect to websocket
 * */
window.onload = function () {
    if (clientId == "") {
        return;
    }

    // Check if the browser supports WebSocket
    if (!window["WebSocket"]) {
        alert("Websockets are not supported");
        return;
    }

    conn = new WebSocket("ws://" + document.location.host + "/ws?clientId=" + clientId + "&clientKind=" + clientKind + "&url=" + url + "&projectName=" + projectName + "&devcardName=" + devcardName);

    conn.addEventListener("close", (event) => {
        console.log(url)
        appendStatusBarContent(" <code class=\"err\">connection lost: <a href=\"javascript:location.reload()\">reload</a></code>")
    });

    conn.onmessage = function(rawMsg) {
        msg = JSON.parse(rawMsg.data)
        dispatchMessage(msg)
    }
};

dispatchMessage = function(msg) {
    switch (msg.msgType) {
    case "clear":
        clearDevcard();
        break;
    case "setTitle":
        setTitle(msg.title);
        break;
    case "appendCell":
        appendCell(msg.cellId)
        setCellContent(msg.cellId, msg.html)
        break;
    case "appendToCell":
        appendToCell(msg.cellId, msg.html)
        break;
    case "setCellContent":
        setCellContent(msg.cellId, msg.html)
        break;
    case "setStatusBarContent":
        setStatusBarContent(msg.html)
        break;
    case "saveScrollPosition":
        scrollPosition = window.pageYOffset
        break;
    case "restoreScrollPosition":
        window.scrollTo(0, scrollPosition);
        break;
    case "jump":
        document.getElementById(msg.id).scrollIntoView();
        break;
    default:
        alert("unsupported message type:" + msg.msgType);
        break;
    }
}

clearDevcard = function() {
    document.getElementById("-devcard-cells").innerHTML = ""
    document.getElementById("-devcard-stdout").innerHTML = ""
    document.getElementById("-devcard-stderr").innerHTML = ""
}

setTitle = function(title) {
    e = document.getElementById("-devcard-title")
    e.innerHTML = '<a style="text-decoration:none" href="javascript:openInEditor()">📂</a> ' +title
}

appendCell = function(cellId) {
    e = document.getElementById(cellId)
    if (e != null) {
        return
    }
    e = document.getElementById("-devcard-cells")
    cell = "<div id=\"" + cellId + "\"></div>"
    e.innerHTML = e.innerHTML + cell
}

setCellContent = function(cellId, html) {
    e = document.getElementById(cellId)
    e.innerHTML = html
}

appendToCell = function(cellId, html) {
    e = document.getElementById(cellId)
    e.innerHTML = e.innerHTML + html
}

setStatusBarContent = function(html) {
    e = document.getElementById("-devcard-status-bar")
    e.innerHTML = html
}

appendStatusBarContent = function(html) {
    e = document.getElementById("-devcard-status-bar")
    e.innerHTML = e.innerHTML + html
}

generateDebugCode = function() {
    fetch("/debug/"+projectName+"/"+devcardName)
        .then((response) => response.text())
        .then((text) => {
            alert(text)
        })
}

openInEditor = function() {
    fetch("/open/"+projectName+"/"+devcardName)
        .then((response) => response.text())
        .then((text) => {
            if (text != "") {
                alert(text)
            }
        })
}
