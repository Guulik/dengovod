# Независимая браузерная приёмка Деньговода

Дата: 2026-11-15, Europe/Moscow. Базовый полный прогон: **32/32 PASS**, 16 desktop + 16 mobile 390px; 0 ошибок, 0 flaky, 0 пропусков, база HEAD `c42b473` с QA-файлами в рабочей копии. Дополнение P2 после CSS fix `6fa8db7`: **2/2 PASS адресно**, desktop + mobile с resize 390→320. История предыдущего мобильного сбоя на `96ed958` сохранена ниже. Единого локального прогона 34 не выполнялось.

## Дополнение P2: «Сегодня» после московской полуночи

Добавлен независимый `frontend/e2e/calendar-midnight.spec.ts` (ещё 2 проверки, по одной в каждом проекте). Seed профиля и якоря выполняется с обычными Node/backend часами. Только browser Date фиксируется через `page.clock.setFixedTime`: `2026-11-30T20:59:00Z` (23:59 Москвы) → вход → переход на октябрь → `2026-11-30T21:01:00Z` (00:01 Москвы 1 декабря) → «Сегодня». Ожидаются заголовок «декабрь 2026» и 1 декабря внутри текущего месяца той же открытой страницы. Таймеры браузера работают в обычном режиме; ожидание двух реальных минут не требуется. API остаётся настоящим, будущие факты не создаются.

`npm run test:e2e:typecheck`: PASS после добавления теста. После frontend fix `96ed95888b28ea197892600c4744acb4453f3296` выполнен только адресный `$env:E2E_BROWSER_CHANNEL='chrome'; npx playwright test e2e/calendar-midnight.spec.ts`: **desktop PASS, mobile-390 FAIL, всего 1/2 PASS**, exit code 1, длительность 41.8 с. Production frontend и Go backend успешно собраны, backend работал с временной SQLite `dengovod-e2e-mqgEZX/test.sqlite`. Промежуточных ошибок сборки backend в этом запуске не было.

Причина исторического mobile FAIL на `96ed958`: шаг «нажать Сегодня» был недоступен пользователю. На viewport 390×844 после перехода на октябрь были видны только стрелки предыдущего/следующего месяца; прежнее правило `frontend/src/index.css:112–114` внутри `@media (max-width: 640px)` задавало `.money-calendar .fc .fc-today-button { display: none; }`. Ожидалось: видимая доступная кнопка «Сегодня» и переход в декабрь после смены browser Date. Фактически: кнопка была скрыта, нажатие не происходило, календарь оставался на октябре. Проверка самого перехода через полночь на desktop была успешна. Это было реальное ограничение мобильного UI, а не ошибка API/clock.

Исторический скриншот: `C:\Users\sasha\Documents\ChatGPT\ДЕньговод\frontend\e2e\.artifacts\history\calendar-midnight-before-css-mobile-390\test-failed-1.png`. Там же сохранены `error-context.md` и `trace.zip` до перезаписи results новым запуском. После прогона в тест добавлено явное ожидание видимости кнопки с пояснением для 390px; действие и ожидание декабря не изменены. Повтор без frontend изменения не выполнялся, скрытая кнопка не нажимается через force/DOM API.

После CSS fix `6fa8db7ee41c5d7c0283f90e1809425ce926a592` выполнен один новый адресный `$env:E2E_BROWSER_CHANNEL='chrome'; npx playwright test e2e/calendar-midnight.spec.ts`: **2/2 PASS**, exit code 0, `2 passed (12.2s)`. Desktop 2.1 с, mobile 2.4 с. В mobile сценарий добавлены resize viewport 390→320, видимость «Сегодня» в viewport, проверка границ кнопки внутри ширины экрана, отсутствие горизонтального переполнения страницы и toolbar, сохранение выбранного декабря. Assertions исходного перехода через московскую полночь не ослаблены. `npm run test:e2e:typecheck` — PASS после дополнения resize.

Независимая проверка ZIP последнего HTML report: `total=2`, `expected=2`, `unexpected=0`, `flaky=0`, `skipped=0`, `ok=true`, duration 12150.957 мс; проекты desktop/mobile-390. `.last-run.json`: passed, failedTests=[]. Backend и production frontend успешно собрались, временная SQLite `dengovod-e2e-zZraAl/test.sqlite` удалена после teardown. Другие 32 зелёных сценария локально не повторялись; единый новый полный набор 34 проверит CI после обоих fixes. Последний HTML report содержит именно адресные 2, исторические 32/32 сохранены в этом отчёте отдельно.

Панели календаря с видимой «Сегодня», проверенные визуально QA (размер изображения соответствует панели внутри указанного viewport):

- viewport 390px: `C:\Users\sasha\Documents\ChatGPT\ДЕньговод\frontend\e2e\.artifacts\visual\calendar-today-390.png`.
- viewport 320px: `C:\Users\sasha\Documents\ChatGPT\ДЕньговод\frontend\e2e\.artifacts\visual\calendar-today-320.png`.

## Изоляция и запуск

- Playwright собирает настоящий Go backend и production frontend во временную папку, затем запускает отдельный Vite preview с proxy к этому backend. В течение прогона проверяется одна сборка; HMR и запись HTML-снимков Playwright не перезагружают приложение. Production src/ и vite.config не изменены QA.
- Каждый запуск создаёт новую `test.sqlite` в каталоге `%TEMP%/dengovod-e2e-*`. `DATABASE_PATH` всегда заменяется сгенерированным путём; рабочая директория backend также временная. Пользовательская БД не открывается.
- Порты по умолчанию: UI `127.0.0.1:4179`, API `127.0.0.1:18089`. Оба проверяются на занятость, `reuseExistingServer=false`. Чужие процессы не останавливаются.
- Штатный teardown вызывает защищённый случайным ключом endpoint своего тестового сервера, закрывает Vite/backend и удаляет только собственный временный каталог. Принудительное завершение Windows может оставить временные файлы.
- На каждый тест отдельный browser context и уникальные профили. Общая временная БД используется только в одном последовательном прогоне.
- Ответы API не подменяются фикстурами. Проверки задержки удерживают реальный ответ. Проверка неопределённого результата подтверждения выполняет реальный PUT, затем обрывает его ответ.
- Desktop: 1440×1000; mobile: 390×844, touch/mobile; timezone Europe/Moscow. Локальный браузер — установленный Chrome **158.0.8127.36** через Playwright **1.65.1**, отдельный временный browser context без пользовательского профиля Chrome. При ошибке сохраняются screenshot/trace, HTML-отчёт — в `frontend/e2e/.artifacts/report/`.

Команды из `frontend`:

```powershell
npm install
npx playwright install chromium
npm run test:e2e
npm run test:e2e:desktop
npm run test:e2e:typecheck
npm run test:e2e:report
```

Опционально другой свободный порт: `$env:E2E_PORT='4180'; $env:E2E_API_PORT='18090'`. Локальный fallback при недоступном CDN браузеров: `$env:E2E_BROWSER_CHANNEL='chrome'; npx playwright test`. Без этой переменной конфигурация использует штатный Playwright Chromium. В CI предусмотрено `npx playwright install --with-deps chromium`.

## Автоматизированное покрытие

`regression.spec.ts`: регистрация, явная настройка и её сохранение после reload; прошлый месяц и полный CRUD без первого якоря; неизвестный баланс; навигация месяца при удержанном реальном ответе и mutation; A → logout → B с выбранным днём и поздним ответом A. MutationObserver проверяет также кратковременное раскрытие суммы/комментария A.

`salary.spec.ts`: 100000 × 15 / 30; явное применение; ручная корректировка результата; сохранение даты выплаты и суммы после reload; уникальные даты и 29 февраля; 28/31 день; half-up до копейки; неверный оклад и число дней; нулевая сумма; сброс несовместимых значений при смене режима. Все сценарии выполняются также на 390px.

`plan-fact.spec.ts`: план 8700 при факте 10000; подтверждение → 18700; заблокированный повтор во время запроса; повтор того же PUT; reload и перенос суммы в следующий день; подтверждение одного месяца серии; снятие факта после изменения recurrence с `plan_restored=false`; неопределённый результат create-as-fact с повтором только того же PUT и без второго POST.

Дополнительно: прошлый факт при неизвестном балансе; начальная сводка с задержкой; резерв 400 и расход 1500 (план → факт → снятие → отмена → восстановление → факт из дневного резерва). При смене профиля также сохраняется доход B с UUID его собственной категории SALARY.

`session-mutation.spec.ts`: настоящий PUT настроек A выполняется в SQLite, но его ответ удерживается; пользователь закрывает настройки, выходит из A, входит в B и заполняет новый редактор. Поздний ответ A не закрывает/сбрасывает черновик B, баланс B остаётся 20000, настройки A=400 и B=0 изолированы. Окно создания события намеренно нельзя закрыть во время POST; тест не обходит эту блокировку. Поздняя мутация проверяется через доступный пользователю путь настроек.

`zz-visual.spec.ts`: перед сохранением мобильных скриншотов проверяются видимые статусы плана/факта, баланс 18700, результат калькулятора 50000 и отсутствие горизонтального переполнения. Скриншоты сохранены на настоящих тестовых данных. Предыдущие изображения техлид визуально проверил; финальный прогон обновил те же пути.

## Команды и результаты

Рабочая директория команд — `C:\Users\sasha\Documents\ChatGPT\ДЕньговод\frontend`.

- `npm install --save-dev @playwright/test`: PASS. Добавлен только разрешённый dev dependency и его lock-записи. npm сообщил 10 vulnerabilities (2 moderate, 8 high); автоматическое обновление остальных зависимостей не выполнялось.
- `npx playwright install chromium`: FAIL по сети, `ECONNRESET` при загрузке CDN и автоматических повторах. Запасная загрузка `npx playwright install chromium --only-shell` остановлена после выбора рабочего Chrome fallback. Это ограничение установки, а не сбой браузерного сценария.
- `npm run test:e2e:typecheck`: PASS, все QA-файлы и конфигурация.
- `npm run lint`: PASS, включая QA-файлы.
- Предыдущий полный `$env:E2E_BROWSER_CHANNEL='chrome'; npx playwright test`: 28 PASS / 4 FAIL. Четыре сбоя были в тестах: слишком точный label после появления ошибки и попытка закрыть намеренно заблокированный редактор. Исправлены локатор и пользовательский путь поздней мутации; финансовые и session assertions не ослаблены.
- Адресный `$env:E2E_BROWSER_CHANNEL='chrome'; npx playwright test --grep 'калькулятор:|поздний PUT'`: **4/4 PASS**. Сохранённый на этом этапе HTML report имел `stats.total=4`; он не выдавался за полный прогон.
- Финальный `$env:E2E_BROWSER_CHANNEL='chrome'; npx playwright test`: **32/32 PASS**, exit code 0, Playwright summary `32 passed (1.4m)`. Начало 2026-11-15 **15:43:21 MSK**, длительность из HTML report **81.407 с**. По файлам: plan/fact 12/12; regression 10/10; salary 6/6; session mutation 2/2; visual 2/2.
- Независимо прочитан ZIP внутри финального `report/index.html`: `total=32`, `expected=32`, `unexpected=0`, `flaky=0`, `skipped=0`, `ok=true`, `errors=[]`; проекты `desktop`, `mobile-390`. `.last-run.json`: `status=passed`, `failedTests=[]`.
- После финального teardown каталог `%TEMP%/dengovod-e2e-nea8Uy` отсутствует, слушателей на тестовых портах 4179/18089 нет. Пользовательская БД не использовалась.

## Артефакты

- HTML report: `C:\Users\sasha\Documents\ChatGPT\ДЕньговод\frontend\e2e\.artifacts\report\index.html`.
- Календарь с планом и фактом, ширина 390px: `C:\Users\sasha\Documents\ChatGPT\ДЕньговод\frontend\e2e\.artifacts\visual\mobile-calendar-plan-fact.png`.
- Калькулятор на viewport 390×844: `C:\Users\sasha\Documents\ChatGPT\ДЕньговод\frontend\e2e\.artifacts\visual\mobile-salary-calculator.png`.

## Изменённые QA-файлы и границы приёмки

- `frontend/package.json`, `frontend/package-lock.json`: dev dependency Playwright, команды e2e/typecheck/report; Vitest исключает e2e через заключённый в двойные кавычки glob, совместимый с Windows и POSIX shell.
- `frontend/playwright.config.ts`.
- `frontend/e2e/.gitignore`, `server.mjs`, `teardown.ts`, `tsconfig.json`, `helpers.ts`.
- `frontend/e2e/plan-fact.spec.ts`, `regression.spec.ts`, `salary.spec.ts`, `session-mutation.spec.ts`, `zz-visual.spec.ts`.
- Дополнение P2: `frontend/e2e/calendar-midnight.spec.ts`.
- `docs/qa/e2e-report.md`.

В базовых 32 браузерных сценариях блокирующих продуктовых дефектов не обнаружено. P2 перехода через московскую полночь и мобильной доступности «Сегодня» проверен после обоих frontend fixes: новые 2 адресных сценария PASS, включая ширины 390 и 320. Локальная приёмка выполнена с Chrome fallback. Этот отчёт не заменяет независимую API/доменную приёмку, ревью и решение техлида о поставке. QA не менял production src/, vite.config, ветки, коммиты, remote и issues.

## Последующая интеграционная проверка техлида

[CI run 43715370852](https://github.com/Guulik/dengovod/actions/runs/43715370852): установка штатного Playwright Chromium и полный e2e на Linux — PASS. Backend с race detector и frontend также прошли. Мобильные скриншоты проверены техлидом визуально.
