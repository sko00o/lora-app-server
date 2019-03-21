import React, { Component } from "react";
import { Link } from "react-router-dom";

import { withStyles } from "@material-ui/core/styles";
import Grid from "@material-ui/core/Grid";
import Button from '@material-ui/core/Button';

import CloudUpload from "mdi-material-ui/CloudUpload";

import Admin from "../../components/Admin";
import theme from "../../theme";


const styles = {
  buttons: {
    textAlign: "right",
  },
  button: {
    marginLeft: 2 * theme.spacing.unit,
  },
  icon: {
    marginRight: theme.spacing.unit,
  },
};


class ListFUOTADeployments extends Component {
  constructor() {
    super();
  }

  render() {
    return(
      <Grid container spacing={24}>
        <Admin organizationID={this.props.match.params.organizationID}>
          <Grid item xs={12} className={this.props.classes.buttons}>
            <Button variant="outlined" className={this.props.classes.button} component={Link} to={`/organizations/${this.props.match.params.organizationID}/applications/${this.props.match.params.applicationID}/devices/${this.props.match.params.devEUI}/fuota-deployments/create`}>
              <CloudUpload className={this.props.classes.icon} />
              Create FUOTA Deployment
            </Button>
          </Grid>
        </Admin>
      </Grid>
    );
  }
}

export default withStyles(styles)(ListFUOTADeployments);
