document.addEventListener("DOMContentLoaded", function (e) {
	var a = document.getElementById("login-link");
	if (a) {
		a.href = a.href + "?callback=" + encodeURIComponent(window.location.href);
	}
	var b = document.getElementById("logout-link");
	if (b) {
		b.href = b.href + "?callback=" + encodeURIComponent(window.location.href);
	}
});
