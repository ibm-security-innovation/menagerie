//
// Copyright 2015 IBM Corp. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

function app() {

    const anyval = "Any";

    var base_url = "/";
    var page_size = 10;
    var max_index = 0;
    var pagination;
    var queues_inited = false;

    if(location.protocol=="file:") {
      base_url = "fixtures/";
      page_size = 2;
    }

    function do_ajax(url, s_callback, e_callback) {
      // console.log("[ajax]", (new Date()).toJSON(), url)
      $.ajax({
        url: base_url + url,
        async: true,
        dataType: 'json',
        timeout: 20*1000,
        success: function (res) { return s_callback(res); },
        error: function(xhr, internal_error, http_status) {
          console.log("%s <%s,%s>",url, internal_error, http_status);
          if(e_callback) e_callback(xhr, internal_error, http_status);
        }
      });
    }

    function jobs_params(min_id) {
      var args = []
      var st = $("#select-status").val();
      if (st && st != anyval) args.push("st=" + st);
      var eng = $("#select-engine").val();
      if (eng && eng != anyval) args.push("eng=" + eng);
      if (min_id > 0) {
        args.push("min-id=" + min_id)
      }
      return args.length ? "?" + args.join("&") : ""
    }

    function load_jobs(max_idx,pg_size,page) {
      do_ajax("monitor/jobs/"+max_idx+"/"+pg_size+"/"+page+jobs_params(), function(data) {
        $("#jobs-list-container").html($.templates("#jobs-list").render(data));
      });
    }

    function update_queues(is_init) {
      do_ajax("monitor/queues", function(data) {
        var stats = [];
        for (var q in data.Queues) {
          var o = data.Queues[q];
          o.engine = q;
          stats.push(o);
        }
        data = {"Stats": stats};
        $("#queues-container").html($.templates("#queues").render(data));
        if (!queues_inited){
          $("#select-engine").html($.templates("#engines").render(data));
          apply_url_state(true);
          queues_inited = true;
        }
      });
    }

    function jobs_refresh() {
      do_ajax("monitor/jobs/paginate"+jobs_params(), function(data) {
        max_index = data.max_index;
        var pages = Math.floor(data.count/page_size) + (((Math.floor(data.count/page_size)*page_size)<data.count) ? 1 : 0);
        if($("#paginator").data().twbsPagination) $("#paginator").data().twbsPagination.destroy();
        if (pages > 0) {
          $('#paginator').twbsPagination({
            totalPages: pages,
            visiblePages: 5,
            onPageClick: function (event, page) {
              load_jobs(max_index,page_size,page);
            },
            first: '<<',
            last: '>>',
            next: '>',
            prev: '<'
          });
        } else {
          load_jobs(0,page_size,1);
        }
        pagination = data;
        $("#refresh-alert").addClass("hidden");
      });
    }

    function refresh_timer() {
      update_queues();
      if (queues_inited) update_jobs();
    }

    function update_jobs() {
      var min_id = pagination ? parseInt(pagination.max_index)+1 : 0;
      do_ajax("monitor/jobs/paginate"+jobs_params(min_id), function(data) {
        if(min_id > 0) {
          var count = parseInt(data.count);
          if(count>0) {
            $("#jobcount").text(count==1 ? "One new job" : count + " new jobs");
            $("#refresh-alert").removeClass("hidden");
          }
        }
      });
    }

    function query_to_dict(q) {
      var d = {};
      var tokens = q.split("&");
      for (var i=0; i<tokens.length; ++i) {
        var token = tokens[i];
        var idx = token.indexOf("=");
        if (idx >= 0) {
          d[token.substr(0, idx)] = token.substr(idx+1);
        }
      }
      return d;
    }

    function set_url_state() {
      var h = "#" + jobs_params();
      if (h != window.location.hash) {
        window.location.hash = h;
      }
    }

    function apply_url_state(force_refresh) {
      var st = anyval, eng = anyval;
      var idx = window.location.hash.indexOf("?");
      if (idx >= 0) {
        var state = query_to_dict(window.location.hash.substr(idx+1));
        st = state["st"] || anyval;
        eng = state["eng"] || anyval;
      }
      var s_eng = $("#select-engine");
      var s_st = $("#select-status");
      var jobs_updated = (s_eng.val() != eng) || (s_st.val() != st);
      $("#select-engine").val(eng);
      $("#select-status").val(st);
      if (jobs_updated || force_refresh) jobs_refresh();
    }

    // initialize elements

    $.material.init();

    $(window).on("hashchange", apply_url_state);

    refresh_timer();

    $("#refresh-alert").click(jobs_refresh);
    $("#select-engine").change(set_url_state);
    $("#select-status").change(set_url_state);

    setInterval(refresh_timer, 10000);

}

$(app);
