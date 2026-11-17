package linuxdo

// Fixtures are trimmed slices of real linux.do RSS documents. Test files are
// exempt from the English-only gate because upstream payloads are data.

const latestFixture = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:discourse="http://www.discourse.org/" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>LINUX DO - 最新话题</title>
    <link>https://linux.do/latest?order=created&amp;page=1</link>
    <description>最新话题</description>
    <language>zh-CN</language>
    <atom:link href="https://linux.do/latest.rss?order=created&amp;page=1" rel="self" type="application/rss+xml" />
    <atom:link href="https://linux.do/latest.md?order=created&amp;page=1" rel="alternate" type="text/markdown" />
      <lastBuildDate>Tue, 17 Nov 2026 08:22:19 +0000</lastBuildDate>
        <item>
          <title>有没有佬买过电热脚垫</title>
          <dc:creator><![CDATA[lililihy]]></dc:creator>
          <category>开发调优</category>
          <description><![CDATA[
            <p>办公室有空调没有地址，脚底下凉飕飕的，电热脚垫管用吗？是单地垫还是要那种半包围的。<br>
<img src="https://cdn.ldstatic.com/images/emoji/twemoji/distorted_face.png?v=16" title=":distorted_face:" class="emoji" alt=":distorted_face:" loading="lazy" width="20" height="20"> 这也能算是开发环境优化吧，属于硬件改动。</p>
            <p><small>8 个帖子 - 5 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3191146">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3191146</link>
          <pubDate>Tue, 17 Nov 2026 08:22:16 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3191146</guid>
          <source url="https://linux.do/t/topic/3191146.rss">有没有佬买过电热脚垫</source>
        </item><item>
          <title>求佬给点多agent协同工作的指点</title>
          <dc:creator><![CDATA[chd44]]></dc:creator>
          <category>开发调优</category>
          <description><![CDATA[
            <p>各位大佬好，最近在研究 Multi-Agent 协作。不同的大模型（Agent）各有长短板，为了最大化利用它们的优势，我目前正在尝试自己搭建一个 Agent Hub。<br>
我当前的配置与思路：</p>
<p>目前订阅了 GPT Pro、Claude Pro 和 Gemini Pro。<br>
架构设想：计划使用逻辑推理能力较强的模型（如 Astra 或 Opus）作为“主控大脑”，由它来负责任务的理解、拆解以及向下游其他模型进行任务路由与分发。</p>
<p>想请教大家几个问题：</p>
<ol>
<li>如何更科学地为不同的 Agent 划分职责，有什么高效的调度与协作策略？</li>
<li>针对这种“主控分发”的架构，大家有没有好的落地思路，或者现成的开源框架/成熟的成品推荐？</li>
</ol>
<p>感谢指教！</p>
            <p><small>2 个帖子 - 2 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3191145">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3191145</link>
          <pubDate>Tue, 17 Nov 2026 08:22:06 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3191145</guid>
          <source url="https://linux.do/t/topic/3191145.rss">求佬给点多agent协同工作的指点</source>
        </item><item>
          <title>最近的Antigrave爽用gemini模型，额度马上见底，日子不好过啊</title>
          <dc:creator><![CDATA[大杰哥]]></dc:creator>
          <category>搞七捻三</category>
          <description><![CDATA[
            <p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/3/6/d/36d5ffa2c9a0b0f11a491674d19835fcf326093b.png" data-download-href="/uploads/short-url/7P6gWljP0LhtLoXkpJEjuk53LAf.png?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/original/4X/3/6/d/36d5ffa2c9a0b0f11a491674d19835fcf326093b.png" alt="image" data-base62-sha1="7P6gWljP0LhtLoXkpJEjuk53LAf" width="526" height="500" data-dominant-color="3D3F49"></a></div>接下来三天怎么过，都是浮云<p></p>
            <p><small>7 个帖子 - 4 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3191136">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3191136</link>
          <pubDate>Tue, 17 Nov 2026 08:20:23 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3191136</guid>
          <source url="https://linux.do/t/topic/3191136.rss">最近的Antigrave爽用gemini模型，额度马上见底，日子不好过啊</source>
        </item>
</channel>
</rss>
`

const topFixture = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:discourse="http://www.discourse.org/" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>LINUX DO - 热门话题</title>
    <link>https://linux.do/top?period=daily</link>
    <description>热门话题</description>
    <language>zh-CN</language>
    <atom:link href="https://linux.do/top.rss?period=daily" rel="self" type="application/rss+xml" />
    <atom:link href="https://linux.do/top.md?period=daily" rel="alternate" type="text/markdown" />
      <lastBuildDate>Tue, 17 Nov 2026 06:07:31 +0000</lastBuildDate>
        <item>
          <title>蹬蹬堂 订阅共享池 公益项目上线了</title>
          <dc:creator><![CDATA[🦐虾佬]]></dc:creator>
          <category>福利羊毛</category>
          <description><![CDATA[
            <h4><a name="p-24845492-h-1" class="anchor" href="https://linux.do#p-24845492-h-1" aria-label="标题链接"></a>本帖使用社区公益推广，符合推广要求。我申明并遵循社区要求的以下内容：</h4>
<ul>
<li><strong>我的项目是免费使用的，无收费（变相收费、赞助）部分：</strong> 是</li>
<li><strong>我的帖子已经打上 <a class="hashtag-cooked" href="https://linux.do/tag/1515-tag/1515" data-type="tag" data-slug="公益推广" data-id="1515" data-style-type="icon" data-icon="tag"><span class="hashtag-icon-placeholder"><svg class="fa d-icon d-icon-square-full svg-icon svg-node"><use href="#square-full"></use></svg></span><span>公益推广</span></a> 标签：</strong> 是</li>
<li><strong>我的项目属于个人项目，与公司或商业机构无关：</strong> 是</li>
<li><strong>我的项目不存在QQ、TG等群组引流：</strong> 是</li>
<li><strong>我的项目不存在非运营必要的网站引流：</strong> 是</li>
<li><strong>我的项目不存在为他人推广、AFF：</strong> 是</li>
<li><strong>我的项目无关联的商业项目：</strong> 是</li>
<li><strong>我的站点存在登录，并已接入 LINUX DO Connect：</strong> 是</li>
<li><strong>我帖子内的项目介绍，AI生成、润色内容部分已截图发出：</strong> 是</li>
<li><strong>以上选择我承诺是永久有效的，接受社区和佬友监督：</strong> 是</li>
</ul>
<p><em>以下为项目介绍正文内容，AI生成、润色内容已使用截图方式发出</em></p>
<hr>
<p>这个项目上周就开始弄了  一直改了很多版都不满意<br>
有很多佬友其实也早就注册使用了，虽然没发出来但是早就发现了<br>
之前也有其他佬友做了叫拾光工坊我就很喜欢  那时候我还拿分享榜第一<br>
现在自己也做了一个类似的   方便大家有更多选择</p>
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/c/b/1/cb1b2f820e981e5bc023f340d358693398e45a49.jpeg" data-download-href="/uploads/short-url/sYLaCBGcwIzb9ftFbS0oaoHYN97.jpeg?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/c/b/1/cb1b2f820e981e5bc023f340d358693398e45a49_2_690x388.jpeg" alt="image" data-base62-sha1="sYLaCBGcwIzb9ftFbS0oaoHYN97" width="690" height="388" srcset="https://cdn3.ldstatic.com/optimized/4X/c/b/1/cb1b2f820e981e5bc023f340d358693398e45a49_2_690x388.jpeg, https://cdn3.ldstatic.com/optimized/4X/c/b/1/cb1b2f820e981e5bc023f340d358693398e45a49_2_1035x582.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/c/b/1/cb1b2f820e981e5bc023f340d358693398e45a49_2_1380x776.jpeg 2x" data-dominant-color="303562"></a></div><p></p>
<p>具体的由我ai助理为大家介绍：<br>
<strong><img src="https://cdn.ldstatic.com/images/emoji/twemoji/shrimp.png?v=16" title=":shrimp:" class="emoji" alt=":shrimp:" loading="lazy" width="20" height="20"> 蹬蹬堂订阅共享池 — 订阅换订阅，签到也能白嫖</strong></p>
<p>手里有闲置订阅不知道干嘛？拿出来分享，赚虾仁。<br>
没订阅又想白嫖？每天签到攒虾仁，换别人的订阅用。</p>
<p>说白了就是个订阅共享池：你分享你的，别人领走，你拿虾仁；你攒够虾仁，也能领别人的。</p>
<p><strong>怎么玩</strong></p>
<ul>
<li>注册：Linux DO 一键登录，送 10 虾仁</li>
<li>签到：每天 +5，连续第二天 +6，第三天起 +8，断签就降档</li>
<li>领取：30 虾仁领一条订阅，30 天有效；到期续领 15 虾仁</li>
<li>分享：TL2 就能分享，别人领一次你得 20 虾仁，48 小时后到账</li>
</ul>
<p><strong>领到的能不能用？</strong></p>
<p>每条分享上架前都会验真：节点数够不够、香港活率、国内能不能用、纯净度，全测一遍才上架；上架后还会定期复验，死了自动下架。所以领到的都是活着的节点，不是僵尸订阅。</p>
<p>要是领到的订阅用不了，点“报失效”，同一天 3 个人举报就自动下架，举报属实举报人还各得 5 虾仁。</p>
<p><strong>现在池子里有什么</strong></p>
<p>主订阅 195 节点，还有 v2raydog、wzm-aio 精选、sunmiao4458 这些，都是验真过的。</p>
<p>地址：<a href="https://l.meta.ai/?u=https%3A%2F%2Fai01.teleapi.top%2Fipcheck%2Fpool&amp;h=AUANmNjnMrjEy3cxCc2f-MsFFH04X8Vg6TTCLmUJn2lnGWH7GSvnf0NRbx0RDD8G5KNyWkQP5lnp7Z0P86H67C9eSFd7xtQiiCYWWiZPx3jifF5-qcWyxA" rel="noopener nofollow ugc">https://ai01.teleapi.top/ipcheck/pool</a><br>
ai部分已截图</p>
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/1/2/d/12d63a96e00c6a4a37c4e721e9d026fe4e866115.jpeg" data-download-href="/uploads/short-url/2GDzOECefBbcchiNG5gewbKw0U5.jpeg?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/1/2/d/12d63a96e00c6a4a37c4e721e9d026fe4e866115_2_530x500.jpeg" alt="image" data-base62-sha1="2GDzOECefBbcchiNG5gewbKw0U5" width="530" height="500" srcset="https://cdn3.ldstatic.com/optimized/4X/1/2/d/12d63a96e00c6a4a37c4e721e9d026fe4e866115_2_530x500.jpeg, https://cdn3.ldstatic.com/optimized/4X/1/2/d/12d63a96e00c6a4a37c4e721e9d026fe4e866115_2_795x750.jpeg 1.5x, https://cdn3.ldstatic.com/original/4X/1/2/d/12d63a96e00c6a4a37c4e721e9d026fe4e866115.jpeg 2x" data-dominant-color="353536"></a></div><p></p>
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/5/c/f/5cfb5845489eef4815b98836953410990e3fa31c.png" data-download-href="/uploads/short-url/dgyqIWBzTJA7mGiWukg8HuPMwVu.png?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/5/c/f/5cfb5845489eef4815b98836953410990e3fa31c_2_423x500.png" alt="image" data-base62-sha1="dgyqIWBzTJA7mGiWukg8HuPMwVu" width="423" height="500" srcset="https://cdn3.ldstatic.com/optimized/4X/5/c/f/5cfb5845489eef4815b98836953410990e3fa31c_2_423x500.png, https://cdn3.ldstatic.com/optimized/4X/5/c/f/5cfb5845489eef4815b98836953410990e3fa31c_2_634x750.png 1.5x, https://cdn3.ldstatic.com/optimized/4X/5/c/f/5cfb5845489eef4815b98836953410990e3fa31c_2_846x1000.png 2x" data-dominant-color="F4F5F3"></a></div><p></p>
<p>青春版、正式版的佬都可以用的哈，把用不完的订阅快过期的订阅拿来发光发热吧~<br>
分享的同时，还能享受到别人的鸡场订阅，妙哉！<br>
有bug或者有更好的建议可以本帖提出来，我喊我的ai工人尽快修改更新迭代！</p>
<p>入口：<a href="https://ai01.teleapi.top/ipcheck/pool" class="inline-onebox" rel="noopener nofollow ugc">蹬蹬堂订阅共享池 · 虾蹬验真</a></p>
            <p><small>67 个帖子 - 63 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3185921">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3185921</link>
          <pubDate>Mon, 16 Nov 2026 09:59:23 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3185921</guid>
          <source url="https://linux.do/t/topic/3185921.rss">蹬蹬堂 订阅共享池 公益项目上线了</source>
        </item><item>
          <title>今天继续分享，我搜集的各种免费网络搜索api</title>
          <dc:creator><![CDATA[loading1]]></dc:creator>
          <category>福利羊毛</category>
          <description><![CDATA[
            <p>前几天分享了一些低价的deepseek渠道，没想到大家有不少感兴趣的，不过格式有点问题导致大家看起来比较困难，这次我分享一些我搜集的免费（白嫖）网络搜索api，这种渠道不少，所以那种一次性的羊毛我很少收藏，一次性的其实有不少呢，今天列出来的基本都是可以循环白嫖的，甚至好多家google登录可以整多倍白嫖额度。<br>
<strong>欢迎大家补充</strong>，我最喜欢羊毛了。<br>
如果大家需要deepseek更详细的渠道，我给大家整理一份。<br>
2026.11.17 补一个，有个佬反馈的agent.tinyfish.ai搜索很好用， <strong>30 次/分钟、500 次/小时</strong>，没有月度总量池——按小时算，一天满打满算上万次</p>
<h1><a name="p-24846954-api-1" class="anchor" href="https://linux.do#p-24846954-api-1" aria-label="标题链接"></a>联网搜索 API 免费额度一览</h1>
<div class="md-table">
<table>
<thead>
<tr>
<th>平台</th>
<th>免费额度</th>
<th>定位</th>
</tr>
</thead>
<tbody>
<tr>
<td>Keenable</td>
<td>免费 10 万次/月/号（多号可叠加）；无 key 也能调，1000 次/小时/IP</td>
<td>免费主力，质量第一档</td>
</tr>
<tr>
<td>知乎开放平台</td>
<td>全网搜索、知乎搜索各 5000 次/天，两池独立</td>
<td>中文新闻实时性强；知乎搜索查经验贴</td>
</tr>
<tr>
<td>AnySearch</td>
<td>免费 1000 次/天/号</td>
<td>英文强；中文小众题有误路由风险</td>
</tr>
<tr>
<td>Linkup</td>
<td>每月自动补满 $20 ≈ 4000 次标准搜索/号</td>
<td>每月续杯型免费层</td>
</tr>
<tr>
<td>Tavily</td>
<td>免费 1000 次/月/号</td>
<td>常用搜索后端</td>
</tr>
<tr>
<td>Exa</td>
<td>每月送 $10 额度 ≈ 2500 次搜索或 1 万页抓取</td>
<td>抓取精检</td>
</tr>
<tr>
<td>百度千帆 AI 搜索</td>
<td>免费 100 次/天</td>
<td>中文补源</td>
</tr>
<tr>
<td>火山豆包搜索</td>
<td>免费 500 次/月</td>
<td>找图专用</td>
</tr>
<tr>
<td>Brave Search API</td>
<td>每月送 $5 credit ≈ 1000 次搜索（需绑卡激活）</td>
<td>独立索引，英文补充</td>
</tr>
<tr>
<td><a href="http://You.com" rel="noopener nofollow ugc">You.com</a></td>
<td>无 key 的 MCP 免费档 100 次/天（长期）</td>
<td>随用随取</td>
</tr>
<tr>
<td>open-websearch</td>
<td>完全免费、自建无 key（会限流）</td>
<td>Web 发现补充层</td>
</tr>
<tr>
<td>Serper</td>
<td>注册送 2500 次一次性</td>
<td>一次性额度</td>
</tr>
<tr>
<td>Serpex</td>
<td>免费 200 credits 一次性</td>
<td>备用</td>
</tr>
<tr>
<td>Parallel</td>
<td>绑支付方式每月送 $5（未核实到账）</td>
<td>英文补充搜索</td>
</tr>
<tr>
<td>元宝搜索（腾讯云 WSA）</td>
<td>每月 2000 次免费为限时活动（2026-11-30 结束），平时 ¥14/千次起</td>
<td>独家可搜公众号</td>
</tr>
<tr>
<td>DeepSeek 官方搜索</td>
<td>无免费，¥5/千次</td>
<td>付费留用</td>
</tr>
</tbody>
</table>
</div>
            <p><small>34 个帖子 - 25 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3186113">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3186113</link>
          <pubDate>Mon, 16 Nov 2026 10:35:06 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3186113</guid>
          <source url="https://linux.do/t/topic/3186113.rss">今天继续分享，我搜集的各种免费网络搜索api</source>
        </item><item>
          <title>难绷，程序员真的是穷途末路了</title>
          <dc:creator><![CDATA[中国人会飞]]></dc:creator>
          <category>搞七捻三</category>
          <description><![CDATA[
            <p>两年前公司有个40多点的大牛离职了<br>
大牛之前在外企干了很多年，能力也是全公司top级别的</p>
<p>今天发现在当保安<br>
实在是有点难绷<br>
程序员的末路真的是保安吗</p>
            <p><small>110 个帖子 - 98 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3186102">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3186102</link>
          <pubDate>Mon, 16 Nov 2026 10:33:02 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3186102</guid>
          <source url="https://linux.do/t/topic/3186102.rss">难绷，程序员真的是穷途末路了</source>
        </item>
</channel>
</rss>
`

const hotFixture = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:discourse="http://www.discourse.org/" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>LINUX DO - 热门话题</title>
    <link>https://linux.do/hot</link>
    <description>热门话题</description>
    <language>zh-CN</language>
    <atom:link href="https://linux.do/hot.rss" rel="self" type="application/rss+xml" />
    <atom:link href="https://linux.do/hot.md" rel="alternate" type="text/markdown" />
      <lastBuildDate>Wed, 11 Nov 2026 03:20:40 +0000</lastBuildDate>
        <item>
          <title>《秘密花园园丁邀请函》</title>
          <dc:creator><![CDATA[Neo]]></dc:creator>
          <category>运营反馈</category>
          <description><![CDATA[
            <p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/d/c/6/dc6900feb2a83459c9962d56ee46e10ec62e4471.png" data-download-href="https://cdn3.ldstatic.com/dc6900feb2a83459c9962d56ee46e10ec62e4471" title="similarweb_2025-08-07_073852_491"><img src="https://cdn3.ldstatic.com/optimized/4X/d/c/6/dc6900feb2a83459c9962d56ee46e10ec62e4471_2_690x350.png" alt="similarweb_2025-08-07_073852_491" data-base62-sha1="vrQ0P4RcwH5LyAPdCCaUY1uimVr" width="690" height="350" srcset="https://cdn3.ldstatic.com/optimized/4X/d/c/6/dc6900feb2a83459c9962d56ee46e10ec62e4471_2_690x350.png, https://cdn3.ldstatic.com/optimized/4X/d/c/6/dc6900feb2a83459c9962d56ee46e10ec62e4471_2_1035x525.png 1.5x, https://cdn3.ldstatic.com/optimized/4X/d/c/6/dc6900feb2a83459c9962d56ee46e10ec62e4471_2_1380x700.png 2x" data-dominant-color="F7F9FB"></a></div><br>
* <em>数据图示来自 SimilarWeb (2025.07)，仅供参考。有关数据已打码，请勿过度联想及讨论。</em><p></p>
<hr>
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/8/5/8/85832a2846a38ee6264a95396e2cf0947c813327.png" data-download-href="https://cdn3.ldstatic.com/85832a2846a38ee6264a95396e2cf0947c813327" title="image"><img src="https://cdn3.ldstatic.com/optimized/4X/8/5/8/85832a2846a38ee6264a95396e2cf0947c813327_2_690x348.png" alt="image" data-base62-sha1="j36woQQITKEWAhyLrMmJrzqkDI3" width="690" height="348" srcset="https://cdn3.ldstatic.com/optimized/4X/8/5/8/85832a2846a38ee6264a95396e2cf0947c813327_2_690x348.png, https://cdn3.ldstatic.com/optimized/4X/8/5/8/85832a2846a38ee6264a95396e2cf0947c813327_2_1035x522.png 1.5x, https://cdn3.ldstatic.com/optimized/4X/8/5/8/85832a2846a38ee6264a95396e2cf0947c813327_2_1380x696.png 2x" data-dominant-color="F7F9FB"></a></div><br>
* <em>数据图示来自 SimilarWeb (2025.08)，仅供参考。有关数据已打码，请勿过度联想及讨论。</em><p></p>
<hr>
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/c/2/f/c2fc637b313935b39af1ab2a11338a2640633402.png" data-download-href="https://cdn3.ldstatic.com/c2fc637b313935b39af1ab2a11338a2640633402" title="image"><img src="https://cdn3.ldstatic.com/optimized/4X/c/2/f/c2fc637b313935b39af1ab2a11338a2640633402_2_690x351.png" alt="image" data-base62-sha1="rOVmTLodlVGsUShuph5zBR87rSq" width="690" height="351" srcset="https://cdn3.ldstatic.com/optimized/4X/c/2/f/c2fc637b313935b39af1ab2a11338a2640633402_2_690x351.png, https://cdn3.ldstatic.com/optimized/4X/c/2/f/c2fc637b313935b39af1ab2a11338a2640633402_2_1035x526.png 1.5x, https://cdn3.ldstatic.com/optimized/4X/c/2/f/c2fc637b313935b39af1ab2a11338a2640633402_2_1380x702.png 2x" data-dominant-color="F8F9FB"></a></div><br>
* <em>数据图示来自 SimilarWeb (2025.09)，仅供参考。有关数据已打码，请勿过度联想及讨论。</em><p></p>
<hr>
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/2/d/f/2df34139bbdd91a146f1fca62faf5d7cc8d52154.png" data-download-href="https://cdn3.ldstatic.com/2df34139bbdd91a146f1fca62faf5d7cc8d52154" title="image"><img src="https://cdn3.ldstatic.com/optimized/4X/2/d/f/2df34139bbdd91a146f1fca62faf5d7cc8d52154_2_690x350.png" alt="image" data-base62-sha1="6yuEK4O6548rQVsSBr2DGrHHdha" width="690" height="350" srcset="https://cdn3.ldstatic.com/optimized/4X/2/d/f/2df34139bbdd91a146f1fca62faf5d7cc8d52154_2_690x350.png, https://cdn3.ldstatic.com/optimized/4X/2/d/f/2df34139bbdd91a146f1fca62faf5d7cc8d52154_2_1035x525.png 1.5x, https://cdn3.ldstatic.com/optimized/4X/2/d/f/2df34139bbdd91a146f1fca62faf5d7cc8d52154_2_1380x700.png 2x" data-dominant-color="F7F9FB"></a></div><br>
* <em>数据图示来自 SimilarWeb (2025.10)，仅供参考。有关数据已打码，请勿过度联想及讨论。</em><p></p>
<hr>
<p>本文未采用AI编写、润色，请各位佬友放心阅读。</p>
<hr>
<p>佬友们，先看看上面的图。这个过去的七月是个不一样的七月，恰逢社区成立一年半的七月，是我们在相关社区中登顶的七月。</p>
<p>这当然值得高兴，但数据毫无意义，因为它只代表过去。数据背后的可爱佬友们，是这个社区最大的价值所在。如果有一点点成就和荣耀，功劳全在每一位佬友！</p>
<p>有人在亮眼的数据面前逐渐疯狂，而我们应该在数据面前冷静。我们需要思考的是，如何让每一次的数据不成为顶峰，而只作为新的起点。未来，才是我们需要聚焦关注的。</p>
<p>这篇帖子，正是我对社区下一个阶段的思考，它将包含并揭示我的思考过程。我之所以将思考过程铺开给大家，是希望大家能理解并检阅我的思考路径。佬友的能力是无限的，如果能指出错误和不足之处，那是我最希望看到的事。</p>
<h2><a name="p-7740154-h-1" class="anchor" href="https://linux.do#p-7740154-h-1" aria-label="标题链接"></a>我先向自己抛出一个终极问题：我们为什么相聚在这里？</h2>
<p>这个问题可能每个人都有自己的答案。但既然大家相聚在这里，就一定存在一个大家都能接受的理由：<strong>这里跟别的地方不一样。</strong></p>
<p>让我们把目光移到社区之外的互联网，我认为可以总结为“四化”：<strong>功利化、情绪化、同质化、娱乐化</strong>。如果你长期在互联网浏览，相信你能对我提出的这“四化”感同身受。</p>
<p>不管是希望对抗或是逃离这样的互联网环境，L站正是那一个不一样的自留地，正如我们的社区愿景：<code>新的理想型社区</code>。</p>
<p>随着大模型的爆发，互联网上的内容被无限地稀释。每天都有海量的内容被生成出来供我们阅读、消遣。我们被困在越来越劣质的内容聚合和推荐的信息茧房中，注意力被无限收割，<strong>其痛一</strong>。</p>
<p>当我们渴望跳出信息茧房，与别人进行有意义的交流和思想连接时，功利和戾气会让你身心俱疲，只想蜷缩在让你片刻舒适的信息茧房中，<strong>其痛二</strong>。</p>
<p>当我们主动跳出来，试图去关注某些大佬，希望能学到一些干货，却发现很多所谓的大佬这里搬那里运，隔三差五还给你灌输焦虑、卖卖课。到头来你在骂骂咧咧中陷入关注、取关的循环，<strong>其痛三</strong>。</p>
<p>此三痛一来，我们是否变成了：有眼不能看，有耳不能听，有口不能言，有脑不能学？</p>
<p>我很少把L站称作论坛，一直更喜欢把它叫做社区。在这个社区里因为真诚，我们可以逃离信息茧房营造的幻象，也少了很多急功近利；因为友善，我们可以敞开心扉与人交流，进行思想连接与碰撞；因为团结，我们不互相攻伐，共同抵御来自四面的污染；因为专业，得以在这个娱乐化的环境里不被稀释。</p>
<p>自由不是你想做什么就能做什么，而是你不想做什么就可以不做什么。社区受益于其所倡导真诚、友善、团结、专业的社区文化，这明确了”我们选择做什么“，但社区更大的价值取决于“我们选择不做什么”。以真诚待人为荣，以虚伪欺人为耻；以友善热心为荣，以傲慢冷漠为耻；以团结协作为荣，以孤立对抗为耻；以专业敬业为荣，以敷衍了事为耻。</p>
<p>所以，这样的社区是值得去好好维护的，也是我做这一切思考的目的所在。维护这样的自留地，也应该是所有社区佬友的共同目标所在。</p>
<h2><a name="p-7740154-h-2" class="anchor" href="https://linux.do#p-7740154-h-2" aria-label="标题链接"></a>基于这样的目标，我给自己抛出了另一个问题：现在的问题是什么？</h2>
<p>如开头所说，顶点是新起点，接下来该怎么做就成了需要直面的首要问题。摆在社区面前有两条很不同的路（恰如去年<a href="https://linux.do/t/topic/188585">向左还是向右的路线问题</a>）：野蛮生长和压实深耕。是选择趁这个势头急速扩张，进一步扩大影响，一味吸纳更多的成员，营造更热闹的声势？还是在热闹面前冷静下来，压实和沉淀，调整找到最佳状态？</p>
<p>这是一对充满辩证的选择。它们可以是正反的对立，也可以是合而为一的合题：社区要保持前进，但前进只能脚踏实地，只为更清晰、更美好的目标前进。<strong>目前，我们选择偏向于压实和沉淀，调整好状态再出发。</strong></p>
<p>理由有以下5个方面：</p>
<ol>
<li>从成本考虑。极速的扩张，带来呈指数增长的成本消耗。无论是从服务器硬件资源，网络带宽资源，还是管理人员的需求上，都会跟不上。</li>
<li>社区没有资本压力，没有盈利指标，也没有融资要求。既然如此，有什么必要自己拿鞭子抽自己？与其盲目，不如看准再出发。</li>
<li>快速扩张加入的成员，缺乏对社区文化的认同和理解，很容易造成文化稀释，甚至是文化冲突。严重破坏社区氛围。</li>
<li>快速扩张会带来更多内容上的水份，这条跟上一条类似，会造成社区内容价值被快速稀释。或形成一个恶性循环。</li>
<li>对于现在这样的社区形态和规模，我没有太多的现成经验可参考。与其冒险激进，不如守护好现有的珍贵。</li>
</ol>
<p><strong>从上面几点可以看出，压实调整是务实且很有必要的选择。如果不能压实调整，社区将会同样出现功利化、情绪化、同质化、娱乐化这“四化”。这无异于丧失了社区存在必要性，毁掉了这块自留地。</strong></p>
<p>一直以来，我在参考”数字花园“这个概念打造L站（不得不说，<code>Discourse</code>这个系统提供的各种工具很好地承载了这一概念）。</p>
<p>我们需要关注的不是如同博客一般的一篇篇文章，也不是如传统论坛那样流水般的信息流。花园之所以是花园，不因为一朵花一棵草，而是整个生态。每一位佬友的思想和观点都在播下一颗颗种子，它们落地、生根、发芽、成长。又彼此关联，相互连接，互相打磨。</p>
<p>举个例子：当佬友们遇到一个AI模型或是一个AI编辑器，会播撒不同的思想种子（背景、出发点、需求），培育出不同的花花草草（帖子、问题、观点），相互引用，相互完善，从而形成花园的美丽一角。再由这一个个美丽一角，构成完整的美丽花园。这是一个逐渐完善的生态和知识系统，这是共建！</p>
<p>一个健康的社区花园，需要每一位佬友的共同照料。每一位佬友就是这个秘密花园的园丁，我们<strong>播种</strong>（发帖询问、教学），<strong>施肥</strong>（点赞优秀内容），<strong>浇水</strong>（解答问题），<strong>除虫</strong>（举报捣乱份子），<strong>除草</strong>（举报有害内容）。在园丁们这样日复一日的悉心照料之下，我们的秘密花园才能美丽、繁盛、有生命力。</p>
<h2><a name="p-7740154-h-3" class="anchor" href="https://linux.do#p-7740154-h-3" aria-label="标题链接"></a>既然新的路线已经明确，那么我们该怎么做？</h2>
<p>结合下阶段目标以及过往运营数据，社区拟在接下来的运营中优化改进的措施有：</p>
<ol>
<li>对自动发帖采取更严厉的处罚。对于自动发帖的是无限期追罚的，并总是采用处理时的处罚力度。</li>
<li>对公告后新产生的偏离话题回复，视为自动发帖处理。你可以在帖子下面发散，但不能毫不相关。</li>
<li>对AI生成文字（包括AI润色）零容忍。这不算一条新规则了，它作为建站之初的“约法三章”级别的存在。它只能截图发出，作为被研究的标本存在，属于花园里的塑料花。</li>
<li>GitHub等开源项目自荐的，不得简单复制粘贴项目README。你需要正经写一篇原创帖子（不得AI编写、润色），告诉佬友项目是做什么的，跟佬友有什么关系，能帮到佬友们什么。必须调整优化格式、图片以做到良好适配社区帖子渲染系统。</li>
<li>博客文章类的，必须在本社区发布全文（可在文末附带原文链接），不能只是部分内容或摘要。社区同样不接受AI编写、润色的内容。你必须要调整优化格式、图片以做到良好适配社区帖子渲染系统。</li>
<li>社区不再接受图床类的图片，请直接使用本站的图片上传功能。你可以直接复制粘贴图片到编辑器，也可以点击上传按钮选择需要上传的图片。</li>
<li>社区鼓励正确选择版块以及使用标签，这将有助于问题解答，彰显你的专业性。社区也鼓励有能力的3级佬友帮忙移动版块，完善标签。</li>
<li>自公告发出后，社区不再接受任何公开形式的AFF（无论版块，无论主帖、回帖，管理发的公开 <a class="hashtag-cooked" href="https://linux.do/tag/%E9%9B%86%E4%B8%AD%E5%B8%96" data-type="tag" data-slug="集中帖" data-id="1534" data-style-type="icon" data-icon="tag"><span class="hashtag-icon-placeholder"><svg class="fa d-icon d-icon-square-full svg-icon svg-node"><use href="#square-full"></use></svg></span><span>集中帖</span></a> 除外），如果你需要对方的AFF，请私信/私聊对方获取。现有的老旧AFF帖，会逐步清理（不会影响账号数据）。</li>
<li>社区将建立 <a class="hashtag-cooked" href="https://linux.do/tag/%E5%8E%9F%E5%88%9B" data-type="tag" data-slug="原创" data-id="1531" data-style-type="icon" data-icon="tag"><span class="hashtag-icon-placeholder"><svg class="fa d-icon d-icon-square-full svg-icon svg-node"><use href="#square-full"></use></svg></span><span>原创</span></a> 标签，供原创帖子使用（非AI生成、润色内容，非洗稿、搬运内容）。社区会不断在 <a class="hashtag-cooked" href="https://linux.do/tag/%E5%8E%9F%E5%88%9B" data-type="tag" data-slug="原创" data-id="1531" data-style-type="icon" data-icon="tag"><span class="hashtag-icon-placeholder"><svg class="fa d-icon d-icon-square-full svg-icon svg-node"><use href="#square-full"></use></svg></span><span>原创</span></a> 标签下挑选内容和格式俱佳的帖子，给予 <a class="hashtag-cooked" href="https://linux.do/tag/%E7%B2%BE%E5%8D%8E%E7%A5%9E%E5%B8%96" data-type="tag" data-slug="精华神帖" data-id="1435" data-style-type="icon" data-icon="tag"><span class="hashtag-icon-placeholder"><svg class="fa d-icon d-icon-square-full svg-icon svg-node"><use href="#square-full"></use></svg></span><span>精华神帖</span></a> 标签，以便让更多佬友看见与认可。</li>
<li>社区会更看重新账号申请人对于社区的理解及认同，拦截更多傲慢甚至捣乱份子的加入。同时确保真正认可这里的新佬友能够成功加入。</li>
</ol>
<p><strong>简单来说，社区对于试图从这里获取流量的帖子，提出了更高的要求。</strong></p>
<p>如果你需要从这里获取流量，则需要好好编写自己的帖子，而非交由AI编写、润色，也不是把一份内容到处复制、粘贴，这是为自己的项目负责，不是交付别人布置的任务。我们认为推广都不用心，很难相信你的项目会用心。还请不要抵触，这是共赢的做法，让佬友看到你的用心，佬友会更认可，你收益会更大。</p>
<p>另外，社区鼓励真实交流，AI写得再好，润色得再华丽，那终究只是你的嘴替，不如你的真情实感和真实文字表达来得珍贵。浓妆和滤镜，带来的只有短暂的新奇和千篇一律的审美疲劳。</p>
<p>对于上述违规，社区将采取循序渐进，温和但坚决的方式处理：</p>
<ol>
<li>首次违规，直接删除或忽略举报式删除，不会影响账号升级数据。</li>
<li>再次违规，采取影响账号升级数据式删除。</li>
<li>恶意违规，直接删除内容并封禁。</li>
</ol>
<p>后续社区还会围绕新的路线，优化并推出更多的举措，以达到最终目标。请放松一点，定规则并处罚不是目的，让佬友们有一个安心、舒心交流的社区才是目的。如果你对于处罚有异议，请直接私信（非私聊）<a class="mention" href="https://linux.do/u/neo">@Neo</a> 或 <a href="https://linux.do/faq">关于页面</a> 中列出的其他管理员，不得直接占用公共资源开帖询问，甚至带节奏。</p>
<p>佬友们，这是一个很重要的十字路口。社区选择了一条更难，但我们坚信更正确的路。这条路，难在需要更多的坚持，难在需要更多的理解，难在需要更多的支持。也许未来一段时间社区的数据不再好看，也许会备受质疑甚至抹黑。</p>
<p>但我相信走过去，社区定能成为一个更好的社区，新的理想型社区。我更相信，一个自知、自律、自省的社区，能真正给每一位成员带来更多的价值提升！这牛皮吹出去，底气那是每一位可爱的佬友给的！</p>
<p>感谢过往日子里每一位管理的辛劳，感谢每一位把L站挂在心里的佬友，也感谢每一位把L站穿在身上的佬友！现诚邀您成为这秘密花园的园丁，真诚、友善、团结、专业，共建你我引以为荣之社区。</p>
            <p><small>5801 个帖子 - 4787 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/847468">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/847468</link>
          <pubDate>Thu, 07 Aug 2025 01:31:00 +0000</pubDate>
          <discourse:topicPinned>Yes</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-847468</guid>
          <source url="https://linux.do/t/topic/847468.rss">《秘密花园园丁邀请函》</source>
        </item><item>
          <title>请不要把互联网上的戾气带来这里！</title>
          <dc:creator><![CDATA[Neo]]></dc:creator>
          <category>运营反馈</category>
          <description><![CDATA[
            <p>是的，L站目前每天都有不少各色各样的佬友加入。对于一个在线社区来说，不断壮大和涌入新的血液是一件好事。</p>
<p>但我每天都要问问自己，这里面有没有问题？真的完全是好事吗？在这个过程中我嗅到了一丝危险的气息：有人试图同质化这里，把这里当作互联网上另一个可以随意发泄情绪的地方！甚至试图占领舆论高地，把这里堂而皇之地变成另一个垃圾场。</p>
<p>这是要万分警惕和坚决予以打击的！L站的愿景是成为新的理想型社区，让每一个一身疲惫的佬友在这里得到放松。哪怕有一刻放松手中攥紧的武器，徜徉在和谐的氛围中得到喘息和治愈。</p>
<p>我和管理们都始终坚定这一点，丝毫不会放松！千里之堤溃于蚁穴，如果任由戾气蔓延，争端四起，最终这里的愿景将会完全破产。所以有病要医，不是同路人不必强行融合。任何把戾气带来这里，试图在这里建立另一个互联网垃圾场的人都是不受欢迎的，都要被驱逐出社区。</p>
<p>请好好说话，友善交流！我们完全支持、鼓励友好交流分享，每个人都可以。键盘是你与人交流分享、互通有无的桥梁，不只是你谋取私利的工具，更不是肆意挥舞用来攻击的武器。</p>
<p>自本公告发布之日起，我们将严肃处理以下3类发言：</p>
<ol>
<li>傲慢轻蔑回复。</li>
<li>阴阳怪气回复。</li>
<li>攻击谩骂回复。</li>
</ol>
<p>如有以上发言，我们将视言论破坏程度采取但不限于：删帖、临时封禁、永久封禁等举措。请各位佬友帮忙积极监督，感谢你们为共建美好社区做出的贡献！请一定一定不要把互联网上的戾气带来这里，这里就要做不一样。</p>
<p>持续时间至最后一个不会好好说话的账号持有者被请出社区为止。</p>
<p>请务必认真、仔细阅读我们的<a href="https://linux.do/faq">社区规则</a>，我们已经把它作为横幅张贴在绝大多数页面（包括发帖编辑器上方）！</p>
<p>请注意：任何举报都需要管理人员手动处理确认，没有任何一条举报是举报人发出后自动通过的。也就是说你收到了举报处理，这意味着管理认同了该举报，认为被举报的内容违规。而随意、恶意的举报不会被通过。</p>
<p>如果你对自己的帖子被举报处理有任何疑问，请查看<a href="https://linux.do/about">管理员列表</a>，点击任何一位管理头像与之私信反馈即可。在与管理沟通之前发帖带节奏的，处理方式一律：删帖+封禁7天起+不予回应。</p>
<p>我们的每次社区规则变动都会更新在这里：<a href="https://linux.do/t/topic/293017" class="inline-onebox">社区公约更新公告 2505221050</a> 并每日提醒你去阅读确认（如果你还没有确认的话）。同时在注册账号时也提醒佬友去阅读并确认社区规则。所以默认各位佬友都是有去阅读知晓的。</p>
<p>我们随时欢迎你在有疑问的时候与我们管理进行沟通，请不要拘谨。避免错误沟通方式带来的误解加深，良好的社区氛围需要你我共同维护。</p>
<p>最后再次呼吁佬友们：</p>
<blockquote>
<p><strong>真诚</strong> 、<strong>友善</strong> 、<strong>团结</strong> 、<strong>专业</strong> ，共建你我引以为荣之社区。</p>
</blockquote>
<p>谢谢！</p>
            <p><small>7979 个帖子 - 6213 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/482293">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/482293</link>
          <pubDate>Sun, 09 Mar 2025 16:13:13 +0000</pubDate>
          <discourse:topicPinned>Yes</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-482293</guid>
          <source url="https://linux.do/t/topic/482293.rss">请不要把互联网上的戾气带来这里！</source>
        </item><item>
          <title>瘦了100斤！！很难，但...我成了！跟佬们分享一下来时路</title>
          <dc:creator><![CDATA[程序员阿鑫]]></dc:creator>
          <category>搞七捻三</category>
          <description><![CDATA[
            <p>篇幅有点长，先给佬们看点 <strong>热血澎湃</strong> 的！<br>
照片就不打码了，这不是我的黑历史，这是我的来时路 <img src="https://cdn.ldstatic.com/images/emoji/twemoji/saluting_face.png?v=16" title=":saluting_face:" class="emoji" alt=":saluting_face:" loading="lazy" width="20" height="20"></p>
<h2><a name="p-24876812-h-1" class="anchor" href="https://linux.do#p-24876812-h-1" aria-label="标题链接"></a>对比图：</h2>
<p>2024年11月 220斤 体脂40+<br>
2026年11月 117斤 体脂11~12<br>
<strong>2年减重103斤</strong><br>
</p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/4/0/5/4056400baf64d9d5589a52faea33f0118c5ec5fb.jpeg" data-download-href="/uploads/short-url/9b9lAIWAbQMzj5bIYCEqEy6HesP.jpeg?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/4/0/5/4056400baf64d9d5589a52faea33f0118c5ec5fb_2_690x182.jpeg" alt="image" data-base62-sha1="9b9lAIWAbQMzj5bIYCEqEy6HesP" width="690" height="182" srcset="https://cdn3.ldstatic.com/optimized/4X/4/0/5/4056400baf64d9d5589a52faea33f0118c5ec5fb_2_690x182.jpeg, https://cdn3.ldstatic.com/optimized/4X/4/0/5/4056400baf64d9d5589a52faea33f0118c5ec5fb_2_1035x273.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/4/0/5/4056400baf64d9d5589a52faea33f0118c5ec5fb_2_1380x364.jpeg 2x" data-dominant-color="7D746C"></a></div><p></p>
<h2><a name="p-24876812-h-2" class="anchor" href="https://linux.do#p-24876812-h-2" aria-label="标题链接"></a>前言（碎碎念）：</h2>
<p>我自己属于从小就胖的类型，初中十几岁的时候就已经胖到了170+，然后后面高中的时候自己通过节食手段控制自己到150（但是体脂很高），哪个时候没有健康饮食的概念，也不知道什么碳蛋脂，只知道饿体重就能瘦。</p>
<p>事因2024年，可能是自己太渴望进步吧，想学习新技术，想要努力挣钱，所以给自己的压力很大，最后通过吃来释放压力，吃东西就会让我很开心，那时候也不忌口，每天就想着吃点什么。每天吃完饭就坐电脑跟前去捣鼓研究一些所谓的“新技术”（对自己而言），熬夜、不运动，肥胖开始找上门… 这时候体重来到了220斤 体脂40+<br>
</p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/4/0/1/401f96993964080f0bc739b1d0e37c6135cfac1a.jpeg" data-download-href="/uploads/short-url/99geBWp93UMQTVGnEgq4nJIjMNQ.jpeg?dl=1" title="E73D33AE-9BD7-494E-9463-62AB91BA8454_1_105_c" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/4/0/1/401f96993964080f0bc739b1d0e37c6135cfac1a_2_499x375.jpeg" alt="E73D33AE-9BD7-494E-9463-62AB91BA8454_1_105_c" data-base62-sha1="99geBWp93UMQTVGnEgq4nJIjMNQ" width="499" height="375" srcset="https://cdn3.ldstatic.com/optimized/4X/4/0/1/401f96993964080f0bc739b1d0e37c6135cfac1a_2_499x375.jpeg, https://cdn3.ldstatic.com/optimized/4X/4/0/1/401f96993964080f0bc739b1d0e37c6135cfac1a_2_748x562.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/4/0/1/401f96993964080f0bc739b1d0e37c6135cfac1a_2_998x750.jpeg 2x" data-dominant-color="7E706A"></a></div><p></p>
<p>说实话，哪个时候一点没有想减肥的念头，只觉得每天吃点什么很快乐。炸串、烧烤、奶茶、火锅、麻辣烫、卷饼…现在回看我感觉自己是被激素控制了，由于胖的太快，肚子上开始出现生长纹（打圈一肚子到腰上）红红的一道子一道子的，刚出现的时候很痒（这里就不放照片了）</p>
<p>事情转机出现在2025年，我想“重生”，我不想这样了，于是长达2年的减肥开始了…</p>
<h2><a name="p-24876812-h-3" class="anchor" href="https://linux.do#p-24876812-h-3" aria-label="标题链接"></a>减肥经过（全是干货，自备水源）</h2>
<p>减肥其实就一句话，我们从小就听：“管住嘴，迈开腿”，其实就是制造热量缺口</p>
<h3><a name="p-24876812-h-1-4" class="anchor" href="https://linux.do#p-24876812-h-1-4" aria-label="标题链接"></a>1、了解自己</h3>
<p>体重高!=胖，体重低!=瘦<br>
有人体重很重，但体脂低（胖瘦子）<br>
有人体重很清，但体脂高（瘦胖子）</p>
<ul>
<li>看什么？
<ul>
<li>看 <strong>体脂率！</strong> 不要一直盯着体重（上称前喝了1斤的水不算是你的体重）</li>
</ul>
</li>
<li>什么时候称？
<ul>
<li>我一般是每天早晨起来上完厕所后去上称</li>
</ul>
</li>
<li>体脂秤准不准？什么牌子比较好/准？
<ul>
<li><strong>没有一个准的！没有一个准的！没有一个准的！</strong> 重要事情说三遍，体脂秤是监控体脂变化，而不是单次，所以说买了体脂秤从开始到最后都使用这个，<strong>别换！别换！别换！</strong> 因为每家的算法不一样，测出来结果不一样。其实道理很好理解：<strong>就是仪器的基准问题，你要以哪个机器的数据做基准，这样的参考才有意义</strong></li>
</ul>
</li>
</ul>
<p>建议买块体脂秤（无广），测一下身体数据。瘦体重占比（肌肉含量）、体脂含量、骨骼等，测一下自己的每天消耗多少</p>
<p>现在的计算公式有2个，我觉得比较有意义：</p>
<ul>
<li>基础代谢（维持生命体征所需的热量）</li>
<li>TDEE（每日总热量消耗）</li>
</ul>
<p>日常计算热量缺口，用TDEE的热量去计算，更精准一些，记得要定期计算一次，因为每次瘦下来后，这些值都会变，这些都是变量，每个月计算一次即可</p>
<h3><a name="p-24876812-h-2-5" class="anchor" href="https://linux.do#p-24876812-h-2-5" aria-label="标题链接"></a>2、饮食控制</h3>
<p>需要先学会看 <strong>配料表</strong>、<strong>营养成分表</strong>、<strong>计算卡路里</strong>、<strong>要知道什人体三大营养素是什么？</strong>、<strong>脂肪燃烧的链路</strong>等等</p>
<ul>
<li>配料表：
<ul>
<li>水、白砂糖开头的 <strong>达咩</strong></li>
<li>包含果葡糖浆、低聚果糖、起酥油、植脂末的等 统统达咩，我让豆包整理了一份</li>
<li><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/e/5/c/e5cbb76f66a8c75df3e28b1942155c3a434752b8.jpeg" data-download-href="/uploads/short-url/wMRNGDwEJgIc97ffy338yoKMiiQ.jpeg?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/e/5/c/e5cbb76f66a8c75df3e28b1942155c3a434752b8_2_375x500.jpeg" alt="image" data-base62-sha1="wMRNGDwEJgIc97ffy338yoKMiiQ" width="375" height="500" srcset="https://cdn3.ldstatic.com/optimized/4X/e/5/c/e5cbb76f66a8c75df3e28b1942155c3a434752b8_2_375x500.jpeg, https://cdn3.ldstatic.com/optimized/4X/e/5/c/e5cbb76f66a8c75df3e28b1942155c3a434752b8_2_562x750.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/e/5/c/e5cbb76f66a8c75df3e28b1942155c3a434752b8_2_750x1000.jpeg 2x" data-dominant-color="E4E4E1"></a></div></li>
</ul>
</li>
<li>营养成分表：
<ul>
<li>KJ也就是千焦，KCAL就是千卡
<ul>
<li>计算卡路里方法就是 KCAL = KJ / 4.2</li>
</ul>
</li>
<li>举例：这个纯牛奶 100ml 312kj，那么每百毫升的卡路里就 312/4.2=74.29kcal，那么这个时候有佬就问了，为什么要计算？想吃点什么就吃什么，吃的干净不就好了？</li>
<li>达咩！不行，你要控制一天的总热量摄入，制造热量缺口<br>
<div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/6/2/a/62a9f878ee5b187fba32853489e4fb5c70aeccb1.png" data-download-href="/uploads/short-url/e4OX37vPXbhP9RCAxTvUBUUkaPf.png?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/original/4X/6/2/a/62a9f878ee5b187fba32853489e4fb5c70aeccb1.png" alt="image" data-base62-sha1="e4OX37vPXbhP9RCAxTvUBUUkaPf" width="285" height="316"></a></div></li>
</ul>
</li>
<li>三大营养素：
<ul>
<li>人体需要三大营养素（缺一不可）：
<ul>
<li>碳水化合物（4kcal/g）</li>
<li>蛋白质（4kcal/g）</li>
<li>脂肪（9kcal/g）</li>
</ul>
</li>
<li>不要极端饮食，甚至节食，三餐都需要吃
<ul>
<li>缺少碳水：emo、掉头发、皮质醇炸、训练没劲</li>
<li>缺少蛋白质：影响伤口愈合速度、肌肉合成、头发合成等、缺了会掉肌肉</li>
<li>缺脂肪：脂溶性维生素无法吸收、睾酮下降</li>
</ul>
</li>
<li>常见食物：
<ul>
<li>碳水：杂豆类（黄豆、黑豆、鹰嘴豆、红豆、绿豆等）、玉米、大米（白米、黑米）、面类（面包、馒头、饼）</li>
<li>蛋白质：
<ul>
<li>动物蛋白：红肉（牛羊肉）、白肉（鸡胸肉鱼类）、牛奶等乳制品</li>
<li>植物蛋白：豆腐、豆浆等等</li>
</ul>
</li>
<li>脂肪：植物油（大豆油、花生油、葵花籽油、菜籽油等等）、动物油（猪油、牛油等等）、坚果</li>
</ul>
</li>
</ul>
</li>
<li>脂肪燃烧的链路：
<ul>
<li>脂肪的消耗是通过二氧化碳+水的方式排除体外的，所以那些穿暴汗服等方式只能让自己脱水，减脂的效果很少，这就是为什么很多佬节食很多天、暴汗一次一顿就能回来的原因</li>
<li>1kg脂肪=7700kcal 为什么不是9000kcal，因为脂肪细胞里面还有水份、细胞等其他的，只算甘油三酯是7700kcal，所以计算卡路里就按7700kcal算即可</li>
</ul>
</li>
</ul>
<p>了解了这些之后，就要开始做饮食计划了，偷懒的可以把你的基础代谢、身高体重等信息给豆包，然后让他给你出一份每天都吃什么</p>
<p>最核心的就是吃能一眼看出来原食材的菜，不要吃深加工的东西，举个例子：</p>
<ul>
<li>我能自己啃个苹果，我就不去榨汁</li>
<li>我能吃大米，我就不去吃年糕</li>
</ul>
<p>减肥前期按上述吃，不加入运动也能瘦，大基数一般能瘦十几斤（水份）</p>
<h3><a name="p-24876812-h-3-6" class="anchor" href="https://linux.do#p-24876812-h-3-6" aria-label="标题链接"></a>3、运动加入</h3>
<h4><a name="p-24876812-h-7" class="anchor" href="https://linux.do#p-24876812-h-7" aria-label="标题链接"></a>先说说我自己</h4>
<ul>
<li>220斤时没有加入任何运动，纯靠饮食瘦到200斤以下</li>
<li>190斤左右买了个公路车，开始每天骑行（短距离时速30左右，骑行30km、60km、100km），周六周天的时候没事骑一次长距离</li>
</ul>
<div class="d-image-grid">
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/c/e/2/ce268438999dfdd2795cdd9898a4e2472d266ad6.jpeg" data-download-href="/uploads/short-url/tpGSz8A5LazYqPqezWS8z8sVmfA.jpeg?dl=1" title="7ecf39cc1ffb6e504b9919a020237ba7" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/c/e/2/ce268438999dfdd2795cdd9898a4e2472d266ad6_2_230x500.jpeg" alt="7ecf39cc1ffb6e504b9919a020237ba7" data-base62-sha1="tpGSz8A5LazYqPqezWS8z8sVmfA" width="230" height="500" srcset="https://cdn3.ldstatic.com/optimized/4X/c/e/2/ce268438999dfdd2795cdd9898a4e2472d266ad6_2_230x500.jpeg, https://cdn3.ldstatic.com/optimized/4X/c/e/2/ce268438999dfdd2795cdd9898a4e2472d266ad6_2_345x750.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/c/e/2/ce268438999dfdd2795cdd9898a4e2472d266ad6_2_460x1000.jpeg 2x" data-dominant-color="21241F"></a></div><br>
<div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/f/e/4/fe49477d81f3d3e510fdfab1cbe2b6e29cd84867.jpeg" data-download-href="/uploads/short-url/Ahwhd1zJV9Ndq3Cqc8ihZL6SPJB.jpeg?dl=1" title="ab5beecf00f62871376eaffa51d8b9e8" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/f/e/4/fe49477d81f3d3e510fdfab1cbe2b6e29cd84867_2_281x375.jpeg" alt="ab5beecf00f62871376eaffa51d8b9e8" data-base62-sha1="Ahwhd1zJV9Ndq3Cqc8ihZL6SPJB" width="281" height="375" srcset="https://cdn3.ldstatic.com/optimized/4X/f/e/4/fe49477d81f3d3e510fdfab1cbe2b6e29cd84867_2_281x375.jpeg, https://cdn3.ldstatic.com/optimized/4X/f/e/4/fe49477d81f3d3e510fdfab1cbe2b6e29cd84867_2_421x562.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/f/e/4/fe49477d81f3d3e510fdfab1cbe2b6e29cd84867_2_562x750.jpeg 2x" data-dominant-color="737575"></a></div><p></p>
</div>
<ul>
<li>150斤左右开始跑步,3km、5km、10km、15km、18km、21.195km逐步往上增加，从刚开始的跑5km喘的受不了，到现在半马147</li>
</ul>
<div class="d-image-grid">
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/f/0/f/f0f64603f36a70ae24867209528f6e3bc0ff93ea.jpeg" data-download-href="/uploads/short-url/ynEelFqtoBM2LLdprMywyWmtat4.jpeg?dl=1" title="492ad4471ef745bb2e9766bac4c020a8" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/f/0/f/f0f64603f36a70ae24867209528f6e3bc0ff93ea_2_230x500.jpeg" alt="492ad4471ef745bb2e9766bac4c020a8" data-base62-sha1="ynEelFqtoBM2LLdprMywyWmtat4" width="230" height="500" srcset="https://cdn3.ldstatic.com/optimized/4X/f/0/f/f0f64603f36a70ae24867209528f6e3bc0ff93ea_2_230x500.jpeg, https://cdn3.ldstatic.com/optimized/4X/f/0/f/f0f64603f36a70ae24867209528f6e3bc0ff93ea_2_345x750.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/f/0/f/f0f64603f36a70ae24867209528f6e3bc0ff93ea_2_460x1000.jpeg 2x" data-dominant-color="BFC0C3"></a></div><br>
<div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/b/9/5/b9592c225adf208b8738e72eb2626b3845b73dd0.jpeg" data-download-href="/uploads/short-url/qrFnKjJXrYl69FBwud7rWmurTry.jpeg?dl=1" title="bda71c7552cf8c3ff76b13d092b02422" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/b/9/5/b9592c225adf208b8738e72eb2626b3845b73dd0_2_230x500.jpeg" alt="bda71c7552cf8c3ff76b13d092b02422" data-base62-sha1="qrFnKjJXrYl69FBwud7rWmurTry" width="230" height="500" srcset="https://cdn3.ldstatic.com/optimized/4X/b/9/5/b9592c225adf208b8738e72eb2626b3845b73dd0_2_230x500.jpeg, https://cdn3.ldstatic.com/optimized/4X/b/9/5/b9592c225adf208b8738e72eb2626b3845b73dd0_2_345x750.jpeg 1.5x, https://cdn3.ldstatic.com/optimized/4X/b/9/5/b9592c225adf208b8738e72eb2626b3845b73dd0_2_460x1000.jpeg 2x" data-dominant-color="EFEFEF"></a></div><p></p>
</div>
<h4><a name="p-24876812-h-8" class="anchor" href="https://linux.do#p-24876812-h-8" aria-label="标题链接"></a>怎么选运动？</h4>
<p>选一个自己喜欢/有意思的运动，游泳、跑步、羽毛球、篮球、乒乓球、骑行、跳绳等等</p>
<p>但如果你是大基数，前期不建议加入剧烈的运动（伤膝盖），可以先控制饮食瘦到一定程度再开始加入活动剧烈的运动，前期还是多出去散散步，快走什么的<br>
「<strong>大基数</strong>」可以选对膝盖友好的运动：</p>
<ul>
<li>爬坡（跑步机坡度调上去，然后走路爬坡，手不要扶/拉跑步机！累了就撑在腰上什么的）</li>
<li>游泳（水的浮力可以避免伤膝盖）</li>
<li>骑行（对膝盖伤害相对较小）</li>
</ul>
<p>「<strong>小基数</strong>」的话就可以直接挑选自己喜欢的运动加进去，小基数更推荐去撸铁，提升基础代谢，靠肌肉去燃脂塑性，而不用大量有氧了</p>
<h4><a name="p-24876812-h-9" class="anchor" href="https://linux.do#p-24876812-h-9" aria-label="标题链接"></a>在选运动的时候有几个知识点：</h4>
<ul>
<li>最大心率：最大心率估算公式：<strong>MHR ≈ 220 − 年龄</strong>（简易版；更准：207 − 0.7× 年龄）通用版不是很准，只能算预估，要想做精准的就去跑间歇自己测</li>
<li>静息心率：身体完全安静、清醒、放松状态下，每分钟心跳次数，跳的越少说明心脏非常强劲（我现在夜间静息38~40左右，大量有氧运动给调整的）</li>
</ul>
<p></p><div class="lightbox-wrapper"><a class="lightbox" href="https://cdn3.ldstatic.com/original/4X/8/b/9/8b9397648c3c5b9a4a3257b7846ab339fb291018.png" data-download-href="/uploads/short-url/jUKA38wZ2MOWcNPGJW1SvUU5PC0.png?dl=1" title="image" rel="noopener nofollow ugc"><img src="https://cdn3.ldstatic.com/optimized/4X/8/b/9/8b9397648c3c5b9a4a3257b7846ab339fb291018_2_690x250.png" alt="image" data-base62-sha1="jUKA38wZ2MOWcNPGJW1SvUU5PC0" width="690" height="250" srcset="https://cdn3.ldstatic.com/optimized/4X/8/b/9/8b9397648c3c5b9a4a3257b7846ab339fb291018_2_690x250.png, https://cdn3.ldstatic.com/original/4X/8/b/9/8b9397648c3c5b9a4a3257b7846ab339fb291018.png 1.5x, https://cdn3.ldstatic.com/original/4X/8/b/9/8b9397648c3c5b9a4a3257b7846ab339fb291018.png 2x" data-dominant-color="F6F6F6"></a></div><p></p>
<p>运动的时候建议买块运动手表，可以更好的检测心率</p>
<h4><a name="p-24876812-h-10" class="anchor" href="https://linux.do#p-24876812-h-10" aria-label="标题链接"></a>局部增肌和全身减脂</h4>
<p><strong>如果各位佬们碰到谁跟你说这个动作瘦肚子，哪个动作瘦腿，你直接给他一巴掌，我瘦了100斤，没有局部瘦，先瘦肚子、胸部、臀腿、脸 这样一点一点瘦的</strong></p>
<p>但是有局部增肌，肌肉是用进废退，不用就会被拆掉，所以要隔断时间就去训练一下，肌肉是坐着不动就在燃烧脂肪的工厂</p>
<h3><a name="p-24876812-h-4-11" class="anchor" href="https://linux.do#p-24876812-h-4-11" aria-label="标题链接"></a>4、谈谈补剂与药品</h3>
<h4><a name="p-24876812-h-41-12" class="anchor" href="https://linux.do#p-24876812-h-41-12" aria-label="标题链接"></a>4.1、补剂</h4>
<p><strong>补剂这些东西因人而异，所谓补剂就是你平时吃的东西不够，才需要补剂，而不是刻板印象健身就要吃蛋白粉</strong><br>
我整理一下我吃过的一些补剂，大家自己选择自己缺乏的东西：</p>
<ul>
<li>肌酸（建议必备）：比较夯，增加运动的爆发力，加速运动后的恢复，不过要每天都吃</li>
<li>鱼油：夯！我不喜欢吃鱼，一周也吃不了几次，所以我会额外补充鱼油，抗炎、降低血脂的，我一般是搭配维生素吃，我现在每天的油脂摄入不够，一些脂溶性维生素我吸收不了，所以搭配着吃</li>
<li>蛋白粉：前期买了一桶乳清分离，蛮贵的，个人感觉不如直接吃肉，我每餐都会大量吃肉（瘦）并不喜欢蛋白粉的口感</li>
<li>辅酶Q10：听说夯，还一直在吃，据说是保障心脏的东西</li>
<li>叶黄素：还可以，护眼的，一直盯着屏幕的需要这个东西</li>
<li>钙镁锌和维生素D3：镁和锌对男生比较好，补钙需要维生素D3参与，这俩需要一起补</li>
<li>左旋肉碱：拉完了，没用，还不如多出去走走</li>
</ul>
<h4><a name="p-24876812-h-42-13" class="anchor" href="https://linux.do#p-24876812-h-42-13" aria-label="标题链接"></a>4.2、药品（特别不推荐！别走这些路）</h4>
<p>注：本次减肥没靠任何药品，这里整理了见过的一些</p>
<ul>
<li>司美格鲁肽：听吃过的同事说副作用很大，降低食欲，会厌食，没劲，整个人浑浑噩噩，原理还是靠的少来减肥，这样只会掉肌肉，降低基础代谢</li>
<li>奥利司他（本人吃过）：家里不是开苦茶子厂的别吃，会喷油，吃了之后就不要相信自己的每一个屁，会阻断后面吃进来的油的吸收，并没有消灭身体里面油的能力</li>
</ul>
<p>药品无法帮助你瘦，但良好的运动习惯可以，佬们还是要多多培养运动的习惯</p>
<h3><a name="p-24876812-h-5-14" class="anchor" href="https://linux.do#p-24876812-h-5-14" aria-label="标题链接"></a>5、谈谈奇怪减脂法</h3>
<ul>
<li>16+8：个人认为不太适合大多数人，16小时禁食放夜间也蛮长的，会饿的难受，我觉得不如三餐都吃，然后控制好自己的饮食内容和量</li>
<li>碳循环：我循环不起来，按照计算，我吃不够碳水，就已经撑的难受了，一不注意就会吃多</li>
<li>生酮饮食：不吃碳水，纯靠脂肪功能，这个我玩不转</li>
<li>液断：没试过，原理是靠吃液体食物，个人感觉很扯，咀嚼东西有时候是种享受（现在拔了智齿更觉得是这样）</li>
</ul>
<h3><a name="p-24876812-h-6-15" class="anchor" href="https://linux.do#p-24876812-h-6-15" aria-label="标题链接"></a>6、奖励餐/放纵餐、平台期</h3>
<p>2年来，我也遇到过卡住不动的情况，也就是所谓的平台期，其实就是 <strong>体重不变、维度变</strong>，需要身体适应体重的维度，你的维度在追体重，不用吃放纵餐，继续保持就行</p>
<p>所谓的放纵餐，其实就是给自己在减肥的道路上的一些奖励，保证1个月1次即可，减肥是以按年为单位的“项目”，人不可能在当下环境下一直保持干净饮食，诱惑很大，中途很容易放弃，所以就需要吃一些好吃的环节一下</p>
<h3><a name="p-24876812-h-7-16" class="anchor" href="https://linux.do#p-24876812-h-7-16" aria-label="标题链接"></a>7、总结</h3>
<p>唠到这里差不多就收尾了，减肥的知识点还有很多，还有很多没有说到的…</p>
<p>两年时间减掉 103 斤，最大的感受就是很爽，之前 220 的时候买衣服都得搜特大码 XXXXXL（5XL），柜员都得去仓库找大码，现在 M 的 L 的，随便从模特身上拿下来就可以穿到自己身上，这点真的很爽。</p>
<p>但比起穿衣自由，更珍贵的是身体状态的变化。从前稍微动一动就气喘吁吁，久坐之后浑身发沉、容易疲惫；现在可以从容跑完半马，静息心率压得很低，精力充沛，不再被肥胖带来的乏力、燥热、自卑裹挟。回头看，减肥从来没有什么一键通关的秘籍，底层逻辑永远绕不开热量缺口，剩下的全是耐心和取舍。</p>
<p>别死死盯着体重秤数字焦虑，体脂、围度、运动能力、精神状态才是更值得跟踪的指标；体脂秤只用来盯趋势，不用纠结单次数值准不准。饮食上拒绝节食自残，三大营养素一个都不能落下，看懂配料表、算清热量，优先原生食材，搞懂脂肪代谢的原理，就不会被暴汗服、排油偏方这类智商税忽悠。大基数稳扎稳打，优先低冲击运动保护关节，等体重回落之后再慢慢上有氧、力量训练；减脂只有全身一起掉，不存在局部瘦，但力量训练保住的肌肉，就是你长期维持身材的底气。</p>
<p>补剂永远只是补充，不是刚需，缺啥再考虑；减脂药物风险重重，代价往往是流失肌肉、压低基础代谢，能不走这条路就尽量不走。网上五花八门的减脂法，16+8、生酮、碳循环这些，别人适配不等于适合你，不用跟风硬啃，能长期坚持的方案才是好方案。平台期也不用恐慌，很多时候只是身体在适应新的体重，维度悄悄在变化，稳住节奏就好；放纵餐不是作弊，是漫长减脂路上的减压阀，控制好频率，给自己一点盼头，才不容易直接崩盘摆烂。</p>
<p>对我而言，这两年的减脂，本质上是一场自我重塑。当初因为压力靠暴食逃避，现在学会和食欲共处，学会读懂自己身体发出的信号。减重不是一场短期冲刺，减到目标体重也不是终点，它是一套需要长期守住的生活方式。</p>
<p>希望各位在减肥路上的佬们理性减脂，放平心态，不要急于求成。慢慢来，稳一点，我们追求的不只是体重数字往下掉，更是拥有掌控自己身体的能力，收获长久的健康和自信。</p>
            <p><small>180 个帖子 - 95 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3190229">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3190229</link>
          <pubDate>Tue, 17 Nov 2026 06:07:31 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3190229</guid>
          <source url="https://linux.do/t/topic/3190229.rss">瘦了100斤！！很难，但...我成了！跟佬们分享一下来时路</source>
        </item>
</channel>
</rss>
`

const categoryFixture = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:discourse="http://www.discourse.org/" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>开发调优 - LINUX DO</title>
    <link>https://linux.do/c/develop/4</link>
    <description>&#39;开发调优&#39; 类别中的话题 此版块包含开发、测试、调试、部署、优化、安全等方面的内容。</description>
    <language>zh-CN</language>
    <atom:link href="https://linux.do/c/develop/4.rss" rel="self" type="application/rss+xml" />
    <atom:link href="https://linux.do/c/develop/4.md" rel="alternate" type="text/markdown" />
      <lastBuildDate>Tue, 17 Nov 2026 08:42:27 +0000</lastBuildDate>
        <item>
          <title>有做个电子宠物的佬友吗？</title>
          <dc:creator><![CDATA[Hewlett Borg]]></dc:creator>
          <category>开发调优</category>
          <description><![CDATA[
            <p>目前想给我app新增一个电子宠物，比如小猫或者小狗，功能有待机状态，微动。也可以喂食、洗澡、玩耍等简单的动作。参考同行他们做的是5*5的那种精灵图（就是一排5个，总共5排）轮播这些画面实现动作效果，但是我用ai生成图片总是达不到想要的要求。请教各位有经验的佬友要怎么做呀。</p>
            <p><small>6 个帖子 - 3 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3191246">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3191246</link>
          <pubDate>Tue, 17 Nov 2026 08:33:37 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3191246</guid>
          <source url="https://linux.do/t/topic/3191246.rss">有做个电子宠物的佬友吗？</source>
        </item><item>
          <title>还有佬在养Hermes的吗？</title>
          <dc:creator><![CDATA[AAAAAAAAAAAA]]></dc:creator>
          <category>开发调优</category>
          <description><![CDATA[
            <p>最近用muse和grok bot比较多，发现这类personal agent还真蛮好用的嘞。</p>
<p>但是muse和bot限制还是比较多，而且muse把我号封了，grok天天给我路由类似GPT的模型，说话太难听了。。。而且bot有点贵了两百多用量还少，要是一直给我用opus5.5我还能接受，别给我路由天天不是不能不能确认的类gpt啊。真扎啤了啊</p>
<p>所以想自己搞一个了。就想着之前搞得Hermes。我没用过这类personal agent类的架构，别的coding agent架构几乎都用过。</p>
<p>所以就像请教一下各位佬，Hermes配置起来难吗，是不是基本功能都全，配置好LLM API、搜索API、自己要用的插件，就可以愉快使用了，还是说需要啥功能得自己搓轮子或者找轮子这样，记忆方面做的是不是不错（刚火的时候就有所耳闻）</p>
<p>然后openclaw 2.0这个重构的版本怎么样呢，和Hermes该怎么选啊。</p>
<p>然后大家的模型一般用什么呢？我主要的需求就是 检索信息、聊天、盯邮箱和日程 ，很简单的任务，但是我想要说话好听的，想用gemini 3.8 flash或者4hyd，dsv4.1flash感觉不会说人话，gpt也不考虑</p>
<p>然后搜索api建议用什么呢？</p>
<p>谢谢各位佬解答啊</p>
            <p><small>15 个帖子 - 11 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3191238">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3191238</link>
          <pubDate>Tue, 17 Nov 2026 08:32:57 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3191238</guid>
          <source url="https://linux.do/t/topic/3191238.rss">还有佬在养Hermes的吗？</source>
        </item><item>
          <title>想知道怎么买vps来搭建节点会比较合适</title>
          <dc:creator><![CDATA[daoge]]></dc:creator>
          <category>开发调优</category>
          <description><![CDATA[
            <p>我之前买过一个无名小厂的东京软银的机器<br>
但是现在再看已经没有了<br>
当时的价格是80r/mon<br>
带宽200 流量500上下</p>
<p>我希望就是带宽在100以上<br>
有三网优化的<br>
韩国日本或者随便什么地方延迟比较低的</p>
<p>目前观察到香港的应该延迟最低，但是宽带的价格很吓人<br>
美国的就很便宜，但是延迟一般要200ms以上</p>
<p>所以，求推荐，发个邀请码也行QAQ</p>
            <p><small>3 个帖子 - 3 位参与者</small></p>
            <p><a href="https://linux.do/t/topic/3191233">阅读完整话题</a></p>
          ]]></description>
          <link>https://linux.do/t/topic/3191233</link>
          <pubDate>Tue, 17 Nov 2026 08:32:36 +0000</pubDate>
          <discourse:topicPinned>No</discourse:topicPinned>
          <discourse:topicClosed>No</discourse:topicClosed>
          <discourse:topicArchived>No</discourse:topicArchived>
          <guid isPermaLink="false">linux.do-topic-3191233</guid>
          <source url="https://linux.do/t/topic/3191233.rss">想知道怎么买vps来搭建节点会比较合适</source>
        </item>
</channel>
</rss>
`
