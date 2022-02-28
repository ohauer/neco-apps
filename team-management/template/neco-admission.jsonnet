local neco_admission_template = import 'neco-admission/main.libsonnet';
local settings = import 'settings.json';
local utility = import 'utility.libsonnet';
utility.prefix_file_names('neco-admission', neco_admission_template(settings))
