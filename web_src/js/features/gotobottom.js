import $ from 'jquery';

export function initGotoBottom() {

    $('#gotop').on("click", function() {
        $(this).hide();
        $('html').animate({scrollTop: 0}, 'slow', function() {
            if (window.issuecomments) {
                $('#godown').show();
                $('#goup').hide();
            }
        });
    });
    $('#gobottom').on("click", function() {
        $(this).hide();
        $('html').animate({scrollTop: document.body.scrollHeight}, 'slow', function() {
            if (window.issuecomments) {
                $('#godown').hide();
                $('#goup').show();
            }
        });
    });
    if ($(window).height() < document.body.scrollHeight)
        $('#gobottom').show();
    else
        $('#gobottom').hide();

    clearTimeout($.data(document.body, 'scrollStopTimer'));
    $.data(document.body, 'scrollStopTimer', setTimeout(function() {
        $(window).on('scroll',function () {
            if ($(window).scrollTop() > $(window).height() / 2)
                $('#gotop').show();
            else
                $('#gotop').hide();
            if (document.body.scrollHeight - $(window).height() - window.scrollY > 10)
                $('#gobottom').show();
            else
                $('#gobottom').hide();
        });
    }));
}